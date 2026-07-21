package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/nz365guy/cb365/internal/auth"
	"github.com/nz365guy/cb365/internal/config"
	"github.com/nz365guy/cb365/internal/graph"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

const (
	teamsDeleteScope     = "ChannelMessage.ReadWrite"
	teamsDeleteGraphHost = "graph.microsoft.com"
	teamsDeleteOperation = "teams.channelMessage.softDelete"
)

var (
	teamIDPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	channelIDPattern = regexp.MustCompile(`^19:[A-Za-z0-9._=-]{1,400}@thread\.tacv2$`)
	messageIDPattern = regexp.MustCompile(`^[0-9]{10,32}$`)
)

type teamsDeleteOptions struct {
	Target  auth.ManagedChannelMessageTarget
	Confirm bool
	DryRun  bool
}

type teamsDeleteHTTPResult struct {
	Attempted     bool
	Status        int
	CorrelationID string
	Err           error
}

type teamsDeleteResult struct {
	Class         string
	Status        int
	CorrelationID string
}

type teamsDeleteDependencies struct {
	loadProfile      func() (*config.Config, string, *config.Profile, error)
	verifyProvenance func(context.Context, *config.Profile, string, auth.ManagedChannelMessageTarget) error
	acquireToken     func(context.Context, *config.Config, *config.Profile) (string, error)
	post             func(context.Context, auth.ManagedChannelMessageTarget, string, bool) teamsDeleteHTTPResult
	consume          func(context.Context, *config.Config, *config.Profile, string, auth.ManagedChannelMessageTarget) error
	audit            func(teamsDeleteAuditEvent) error
	now              func() time.Time
}

type teamsDeleteAuditEvent struct {
	Timestamp      string `json:"timestamp"`
	Operation      string `json:"operation"`
	TenantID       string `json:"tenantId"`
	Profile        string `json:"profilePseudonym"`
	TeamID         string `json:"teamId"`
	ChannelID      string `json:"channelId"`
	MessageID      string `json:"messageId"`
	ResultClass    string `json:"resultClass"`
	HTTPStatus     int    `json:"httpStatus,omitempty"`
	CorrelationID string `json:"graphCorrelationId,omitempty"`
}

type teamsDeleteFailure struct {
	class   string
	message string
}

func (e *teamsDeleteFailure) Error() string { return e.message }

var productionTeamsDeleteDependencies = teamsDeleteDependencies{
	loadProfile:      loadSelectedProfile,
	verifyProvenance: auth.VerifyManagedChannelMessageProvenance,
	acquireToken:     acquireTeamsDeleteToken,
	post:             postTeamsDeleteOnce,
	consume:          consumeTeamsDeleteProvenance,
	audit:            appendTeamsDeleteAudit,
	now:              time.Now,
}

var teamsChannelsDeleteMessageCmd = &cobra.Command{
	Use:   "delete-message",
	Short: "Soft-delete one cb365-authored Teams root channel message",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		teamID, _ := cmd.Flags().GetString("team")
		channelID, _ := cmd.Flags().GetString("channel")
		messageID, _ := cmd.Flags().GetString("message")
		confirm, _ := cmd.Flags().GetBool("confirm")
		options := teamsDeleteOptions{
			Target: auth.ManagedChannelMessageTarget{
				TeamID:    teamID,
				ChannelID: channelID,
				MessageID: messageID,
			},
			Confirm: confirm,
			DryRun:  flagDryRun,
		}
		result, err := executeTeamsDelete(cmd.Context(), options, productionTeamsDeleteDependencies)
		if err != nil {
			return err
		}
		if options.DryRun {
			if flagJSON {
				return output.JSON(map[string]interface{}{
					"dryRun":  true,
					"team":    options.Target.TeamID,
					"channel": options.Target.ChannelID,
					"message": options.Target.MessageID,
				})
			}
			output.Info(fmt.Sprintf("[DRY RUN] Would soft-delete message %s in channel %s of team %s", options.Target.MessageID, options.Target.ChannelID, options.Target.TeamID))
			return nil
		}
		if flagJSON {
			return output.JSON(map[string]interface{}{
				"result":         result.Class,
				"team":           options.Target.TeamID,
				"channel":        options.Target.ChannelID,
				"message":        options.Target.MessageID,
				"httpStatus":     result.Status,
				"correlationId":  result.CorrelationID,
			})
		}
		output.Success(fmt.Sprintf("Message %s soft-deleted (HTTP %d)", options.Target.MessageID, result.Status))
		return nil
	},
}

func executeTeamsDelete(ctx context.Context, options teamsDeleteOptions, deps teamsDeleteDependencies) (teamsDeleteResult, error) {
	if err := validateTeamsDeleteTarget(options.Target); err != nil {
		return teamsDeleteResult{}, err
	}
	if !options.Confirm {
		return teamsDeleteResult{}, &teamsDeleteFailure{class: "confirmation_required", message: "--confirm is required to soft-delete this exact message"}
	}
	if options.DryRun {
		return teamsDeleteResult{Class: "dry_run"}, nil
	}
	if deps.loadProfile == nil || deps.verifyProvenance == nil || deps.acquireToken == nil ||
		deps.post == nil || deps.consume == nil || deps.audit == nil || deps.now == nil {
		return teamsDeleteResult{}, errors.New("delete-message dependencies are incomplete")
	}
	cfg, _, profile, err := deps.loadProfile()
	if err != nil {
		return teamsDeleteResult{}, err
	}
	if err := validateTeamsDeleteProfile(profile); err != nil {
		return finishTeamsDelete(deps, profile, options.Target, teamsDeleteResult{Class: "profile_rejected"}, err)
	}
	key := auth.ManagedChannelMessageProvenanceKey(options.Target)
	reference := profile.ManagedDelegated.ChannelMessageProvenance[key]
	if reference == "" {
		return finishTeamsDelete(deps, profile, options.Target, teamsDeleteResult{Class: "provenance_missing"},
			&teamsDeleteFailure{class: "provenance_missing", message: "message is not recorded as a root channel message sent by this managed profile"})
	}
	if err := deps.verifyProvenance(ctx, profile, reference, options.Target); err != nil {
		return finishTeamsDelete(deps, profile, options.Target, teamsDeleteResult{Class: "provenance_rejected"},
			&teamsDeleteFailure{class: "provenance_rejected", message: "managed own-message provenance could not be verified"})
	}
	token, err := deps.acquireToken(ctx, cfg, profile)
	if err != nil {
		return finishTeamsDelete(deps, profile, options.Target, teamsDeleteResult{Class: "authentication_failed"},
			&teamsDeleteFailure{class: "authentication_failed", message: "managed delegated authentication failed; re-authenticate the selected profile"})
	}
	httpResult := deps.post(ctx, options.Target, token, auth.ShouldUseIPv4(cfg))
	token = ""
	result, resultErr := classifyTeamsDeleteHTTP(httpResult)
	if httpResult.Attempted {
		if consumeErr := deps.consume(context.Background(), cfg, profile, reference, options.Target); consumeErr != nil {
			result.Class = "provenance_retirement_failed"
			resultErr = &teamsDeleteFailure{class: result.Class, message: "delete outcome recorded but provenance retirement failed; do not retry this message"}
		}
	}
	return finishTeamsDelete(deps, profile, options.Target, result, resultErr)
}

func finishTeamsDelete(
	deps teamsDeleteDependencies,
	profile *config.Profile,
	target auth.ManagedChannelMessageTarget,
	result teamsDeleteResult,
	resultErr error,
) (teamsDeleteResult, error) {
	tenantID := ""
	profilePseudonym := ""
	if profile != nil {
		tenantID = profile.TenantID
		profilePseudonym = teamsDeleteProfilePseudonym(profile)
	}
	event := teamsDeleteAuditEvent{
		Timestamp:      deps.now().UTC().Format(time.RFC3339Nano),
		Operation:      teamsDeleteOperation,
		TenantID:       tenantID,
		Profile:        profilePseudonym,
		TeamID:         target.TeamID,
		ChannelID:      target.ChannelID,
		MessageID:      target.MessageID,
		ResultClass:    result.Class,
		HTTPStatus:     result.Status,
		CorrelationID: result.CorrelationID,
	}
	if auditErr := deps.audit(event); auditErr != nil {
		if result.Status != 0 || result.Class == "ambiguous" {
			return result, &teamsDeleteFailure{class: "audit_failed", message: "delete outcome was not durably audited; do not retry this message"}
		}
		return result, &teamsDeleteFailure{class: "audit_failed", message: "delete-message audit write failed before any Graph request"}
	}
	return result, resultErr
}

func validateTeamsDeleteTarget(target auth.ManagedChannelMessageTarget) error {
	if !teamIDPattern.MatchString(target.TeamID) {
		return &teamsDeleteFailure{class: "invalid_target", message: "--team must be one exact team GUID"}
	}
	if !channelIDPattern.MatchString(target.ChannelID) {
		return &teamsDeleteFailure{class: "invalid_target", message: "--channel must be one exact Teams channel ID ending in @thread.tacv2"}
	}
	if !messageIDPattern.MatchString(target.MessageID) {
		return &teamsDeleteFailure{class: "invalid_target", message: "--message must be one exact numeric root-message ID"}
	}
	return nil
}

func validateTeamsDeleteProfile(profile *config.Profile) error {
	if profile == nil || profile.AuthMode != config.AuthModeDelegated {
		return &teamsDeleteFailure{class: "profile_rejected", message: "delete-message requires a delegated work-or-school profile"}
	}
	if profile.ManagedDelegated == nil || profile.ManagedDelegated.MigrationState != "complete" {
		return &teamsDeleteFailure{class: "profile_rejected", message: "delete-message requires the BWS EU managed delegated profile; complete auth migration first"}
	}
	hasRequired := false
	for _, scope := range profile.Scopes {
		normalized := strings.TrimSpace(scope)
		if index := strings.LastIndex(normalized, "/"); index >= 0 {
			normalized = normalized[index+1:]
		}
		switch strings.ToLower(normalized) {
		case strings.ToLower(teamsDeleteScope):
			hasRequired = true
		case "channelmessage.read.all", "group.read.all", "group.readwrite.all":
			return &teamsDeleteFailure{class: "profile_rejected", message: "delete-message refuses profiles with broader message-read or group permissions"}
		}
	}
	if !hasRequired {
		return &teamsDeleteFailure{class: "profile_rejected", message: "selected delegated profile is missing ChannelMessage.ReadWrite"}
	}
	return nil
}

func acquireTeamsDeleteToken(ctx context.Context, cfg *config.Config, profile *config.Profile) (string, error) {
	credential, err := auth.NewManagedDelegatedCredential(profile, auth.ShouldUseIPv4(cfg))
	if err != nil {
		return "", err
	}
	token, err := credential.GetToken(ctx, policy.TokenRequestOptions{
		EnableCAE: true,
		Scopes:    auth.GraphScopes([]string{teamsDeleteScope}),
	})
	if err != nil {
		return "", err
	}
	return token.Token, nil
}

func postTeamsDeleteOnce(ctx context.Context, target auth.ManagedChannelMessageTarget, token string, ipv4Only bool) teamsDeleteHTTPResult {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if ipv4Only {
		transport = graph.NewIPv4Transport()
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return postTeamsDeleteOnceWithClient(ctx, target, token, "https://"+teamsDeleteGraphHost+"/v1.0", client)
}

func postTeamsDeleteOnceWithClient(ctx context.Context, target auth.ManagedChannelMessageTarget, token, baseURL string, client *http.Client) teamsDeleteHTTPResult {
	endpoint := strings.TrimRight(baseURL, "/") + "/teams/" + url.PathEscape(target.TeamID) +
		"/channels/" + url.PathEscape(target.ChannelID) + "/messages/" + url.PathEscape(target.MessageID) + "/softDelete"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil || client == nil {
		return teamsDeleteHTTPResult{Err: errors.New("construct Graph soft-delete request")}
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return teamsDeleteHTTPResult{Attempted: true, Err: errors.New("Graph soft-delete transport outcome is ambiguous")}
	}
	defer response.Body.Close()
	correlation := response.Header.Get("request-id")
	if correlation == "" {
		correlation = response.Header.Get("client-request-id")
	}
	return teamsDeleteHTTPResult{Attempted: true, Status: response.StatusCode, CorrelationID: correlation}
}

func classifyTeamsDeleteHTTP(httpResult teamsDeleteHTTPResult) (teamsDeleteResult, error) {
	result := teamsDeleteResult{Status: httpResult.Status, CorrelationID: httpResult.CorrelationID}
	if httpResult.Err != nil {
		if httpResult.Attempted {
			result.Class = "ambiguous"
			return result, &teamsDeleteFailure{class: result.Class, message: "Graph soft-delete outcome is ambiguous; do not retry, perform a separate read-back check"}
		}
		result.Class = "request_rejected"
		return result, &teamsDeleteFailure{class: result.Class, message: "Graph soft-delete request could not be constructed"}
	}
	switch httpResult.Status {
	case http.StatusNoContent:
		result.Class = "success"
		return result, nil
	case http.StatusUnauthorized:
		result.Class = "unauthorized"
	case http.StatusForbidden:
		result.Class = "forbidden"
	case http.StatusNotFound:
		result.Class = "not_found"
	default:
		result.Class = "graph_error"
	}
	return result, &teamsDeleteFailure{class: result.Class, message: fmt.Sprintf("Graph soft-delete failed with redacted result %s (HTTP %d); do not retry this message", result.Class, result.Status)}
}

func consumeTeamsDeleteProvenance(
	ctx context.Context,
	cfg *config.Config,
	profile *config.Profile,
	reference string,
	target auth.ManagedChannelMessageTarget,
) error {
	if err := auth.DeleteManagedChannelMessageProvenance(ctx, profile, reference, target); err != nil {
		return err
	}
	delete(profile.ManagedDelegated.ChannelMessageProvenance, auth.ManagedChannelMessageProvenanceKey(target))
	return cfg.Save()
}

func teamsDeleteProfilePseudonym(profile *config.Profile) string {
	sum := sha256.Sum256([]byte(profile.TenantID + "\x00" + profile.Name))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func appendTeamsDeleteAudit(event teamsDeleteAuditEvent) error {
	directory, err := config.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	path := filepath.Join(directory, "teams-delete-audit.jsonl")
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return errors.New("unsafe Teams delete audit path")
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600) // #nosec G304 -- fixed filename under the cb365 config directory
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.New("unsafe Teams delete audit file")
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := file.Write(encoded); err != nil {
		return err
	}
	return file.Sync()
}
