package main

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

var mailTranslateID, mailTranslateSource, mailTranslateTarget string
var exchangeIdentifier = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]+$`)

func validExchangeIdentifier(identifier string) bool {
	return len(identifier) > 0 && len(identifier) <= 2048 && exchangeIdentifier.MatchString(identifier)
}

func newTranslateIDRequest(identifier, source, target string) (*users.ItemTranslateExchangeIdsPostRequestBody, error) {
	if !validExchangeIdentifier(identifier) {
		return nil, fmt.Errorf("--id must be one valid Exchange identifier")
	}
	sourceValue, _ := models.ParseExchangeIdFormat(source)
	targetValue, _ := models.ParseExchangeIdFormat(target)
	if sourceValue == nil || targetValue == nil {
		return nil, fmt.Errorf("unsupported Exchange identifier format")
	}
	if source == target {
		return nil, fmt.Errorf("source and target formats must differ")
	}
	request := users.NewItemTranslateExchangeIdsPostRequestBody()
	request.SetInputIds([]string{identifier})
	request.SetSourceIdType(sourceValue.(*models.ExchangeIdFormat))
	request.SetTargetIdType(targetValue.(*models.ExchangeIdFormat))
	return request, nil
}

func translatedMessageID(identifier string, values []models.ConvertIdResultable) (string, error) {
	if len(values) != 1 || values[0] == nil || deref(values[0].GetSourceId()) != identifier || !validExchangeIdentifier(deref(values[0].GetTargetId())) {
		return "", fmt.Errorf("identifier translation was not confirmed")
	}
	return deref(values[0].GetTargetId()), nil
}

var mailTranslateIDCmd = &cobra.Command{
	Use:   "translate-id",
	Short: "Translate one Exchange message identifier without modifying mail",
	RunE: func(cmd *cobra.Command, args []string) error {
		request, err := newTranslateIDRequest(mailTranslateID, mailTranslateSource, mailTranslateTarget)
		if err != nil {
			return err
		}
		client, err := newGraphClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		response, err := client.Me().TranslateExchangeIds().PostAsTranslateExchangeIdsPostResponse(ctx, request, nil)
		if err != nil {
			return fmt.Errorf("translating identifier failed")
		}
		if response == nil {
			return fmt.Errorf("identifier translation was not confirmed")
		}
		target, err := translatedMessageID(mailTranslateID, response.GetValue())
		if err != nil {
			return err
		}
		if output.Resolve(flagJSON, flagPlain) == output.FormatJSON {
			return output.JSON(map[string]string{"source_id": mailTranslateID, "target_id": target,
				"source_type": mailTranslateSource, "target_type": mailTranslateTarget})
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", mailTranslateID, target)
		return err
	},
}

func init() {
	mailTranslateIDCmd.Flags().StringVar(&mailTranslateID, "id", "", "Existing message identifier")
	mailTranslateIDCmd.Flags().StringVar(&mailTranslateSource, "source-type", "restId", "Source Exchange identifier format")
	mailTranslateIDCmd.Flags().StringVar(&mailTranslateTarget, "target-type", "restImmutableEntryId", "Target Exchange identifier format")
	mailCmd.AddCommand(mailTranslateIDCmd)
}
