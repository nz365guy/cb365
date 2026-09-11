package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

type mailDeltaFolderState struct {
	DeltaLink         string `json:"delta_link,omitempty"`
	LastSyncAt        string `json:"last_sync_at,omitempty"`
	LastCycleComplete bool   `json:"last_cycle_complete"`
	LastCyclePages    int    `json:"last_cycle_pages"`
	LastCycleItems    int    `json:"last_cycle_items"`
	LastIncompleteAt  string `json:"last_incomplete_at,omitempty"`
}

type mailDeltaState struct {
	Version int                             `json:"version"`
	Folders map[string]mailDeltaFolderState `json:"folders"`
}

var (
	mailDeltaFolderID  string
	mailDeltaStatePath string
	mailDeltaMax       int32
)

func defaultMailDeltaStatePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cb365-mail-delta-state.json")
	}
	return filepath.Join(configDir, "cb365", "mail-delta-state.json")
}

func loadMailDeltaState(path string) (mailDeltaState, error) {
	state := mailDeltaState{Version: 1, Folders: map[string]mailDeltaFolderState{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read delta state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse delta state: %w", err)
	}
	if state.Version != 1 || state.Folders == nil {
		return state, fmt.Errorf("unsupported delta state format")
	}
	return state, nil
}

func saveMailDeltaState(path string, state mailDeltaState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create delta state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode delta state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mail-delta-state-*")
	if err != nil {
		return fmt.Errorf("create delta state temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protect delta state: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write delta state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close delta state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit delta state: %w", err)
	}
	return nil
}

func mailDeltaTokenExpired(err error) bool {
	detail := strings.ToLower(err.Error())
	for _, marker := range []string{
		"invaliddatetimetoken",
		"invalid delta token",
		"syncstatenotfound",
		"resourcenotfound",
		"http status code 410",
		"status code: 410",
	} {
		if strings.Contains(detail, marker) {
			return true
		}
	}
	return false
}

func mailDeltaQueryParameters() *users.ItemMailFoldersItemMessagesDeltaRequestBuilderGetQueryParameters {
	return &users.ItemMailFoldersItemMessagesDeltaRequestBuilderGetQueryParameters{
		Top: &mailDeltaMax,
		Select: []string{
			"id", "subject", "from", "toRecipients", "receivedDateTime",
			"lastModifiedDateTime", "isRead", "hasAttachments", "importance",
			"bodyPreview", "internetMessageHeaders",
		},
	}
}

var mailDeltaFolderCmd = &cobra.Command{
	Use:   "delta-folder",
	Short: "Synchronize folder message changes with a persisted Graph delta link",
	RunE: func(cmd *cobra.Command, args []string) error {
		folderID := strings.TrimSpace(mailDeltaFolderID)
		if folderID == "" {
			return fmt.Errorf("--folder is required")
		}
		if mailDeltaMax < 1 || mailDeltaMax > 1000 {
			return fmt.Errorf("--max must be between 1 and 1000")
		}

		statePath := strings.TrimSpace(mailDeltaStatePath)
		if statePath == "" {
			statePath = defaultMailDeltaStatePath()
		}
		allState, err := loadMailDeltaState(statePath)
		if err != nil {
			return err
		}
		folderState := allState.Folders[folderID]
		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		messages, deltaLink, pages, err := fetchMailDelta(ctx, client, folderID, folderState.DeltaLink)
		if err != nil && folderState.DeltaLink != "" && mailDeltaTokenExpired(err) {
			messages, deltaLink, pages, err = fetchMailDelta(ctx, client, folderID, "")
		}
		if err != nil {
			folderState.LastCycleComplete = false
			folderState.LastIncompleteAt = time.Now().UTC().Format(time.RFC3339)
			folderState.LastCyclePages = pages
			folderState.LastCycleItems = len(messages)
			allState.Folders[folderID] = folderState
			if stateErr := saveMailDeltaState(statePath, allState); stateErr != nil {
				return fmt.Errorf("delta cycle failed: %v; recording incomplete metric failed: %w", err, stateErr)
			}
			return fmt.Errorf("delta cycle incomplete after %d page(s): %w", pages, err)
		}
		if deltaLink == "" {
			return fmt.Errorf("delta cycle incomplete: Graph returned no delta link")
		}

		folderState.DeltaLink = deltaLink
		folderState.LastSyncAt = time.Now().UTC().Format(time.RFC3339)
		folderState.LastCycleComplete = true
		folderState.LastCyclePages = pages
		folderState.LastCycleItems = len(messages)
		folderState.LastIncompleteAt = ""
		allState.Folders[folderID] = folderState
		if err := saveMailDeltaState(statePath, allState); err != nil {
			return err
		}

		if flagJSON {
			items := make([]map[string]interface{}, 0, len(messages))
			for _, msg := range messages {
				items = append(items, formatMessageJSON(msg))
			}
			return output.JSON(items)
		}
		rows := make([][]string, 0, len(messages))
		for _, msg := range messages {
			from := ""
			if msg.GetFrom() != nil {
				from = recipientString(msg.GetFrom())
			}
			rows = append(rows, []string{from, deref(msg.GetSubject()), deref(msg.GetId())})
		}
		if flagPlain {
			output.Plain(rows)
		} else {
			output.Table([]string{"FROM", "SUBJECT", "ID"}, rows)
		}
		return nil
	},
}

func fetchMailDelta(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, folderID, deltaLink string) ([]models.Messageable, string, int, error) {
	var response users.ItemMailFoldersItemMessagesDeltaGetResponseable
	var err error
	builder := client.Me().MailFolders().ByMailFolderId(folderID).Messages().Delta()
	if deltaLink != "" {
		response, err = builder.WithUrl(deltaLink).GetAsDeltaGetResponse(ctx, nil)
	} else {
		response, err = builder.GetAsDeltaGetResponse(ctx, &users.ItemMailFoldersItemMessagesDeltaRequestBuilderGetRequestConfiguration{QueryParameters: mailDeltaQueryParameters()})
	}
	if err != nil {
		return nil, "", 0, fmt.Errorf("fetching folder delta: %w", err)
	}
	if response == nil {
		return nil, "", 0, fmt.Errorf("Graph returned an empty delta response")
	}
	messages := make([]models.Messageable, 0)
	pages := 0
	for response != nil {
		pages++
		messages = append(messages, response.GetValue()...)
		next := response.GetOdataNextLink()
		if next == nil || strings.TrimSpace(*next) == "" {
			if link := response.GetOdataDeltaLink(); link != nil {
				return messages, strings.TrimSpace(*link), pages, nil
			}
			return messages, "", pages, nil
		}
		response, err = builder.WithUrl(strings.TrimSpace(*next)).GetAsDeltaGetResponse(ctx, nil)
		if err != nil {
			return messages, "", pages, fmt.Errorf("follow delta continuation: %w", err)
		}
	}
	return messages, "", pages, fmt.Errorf("Graph returned an empty delta page")
}

func init() {
	mailDeltaFolderCmd.Flags().StringVar(&mailDeltaFolderID, "folder", "", "Folder ID or well-known folder name")
	mailDeltaFolderCmd.Flags().StringVar(&mailDeltaStatePath, "state-file", "", "Path to the folder-scoped delta state file")
	mailDeltaFolderCmd.Flags().Int32Var(&mailDeltaMax, "max", 100, "Preferred page size")
	mailCmd.AddCommand(mailDeltaFolderCmd)
}
