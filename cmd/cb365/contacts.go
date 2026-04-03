package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"github.com/nz365guy/cb365/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────
//  Contacts helpers
// ──────────────────────────────────────────────

// contactEmailsString returns a comma-separated list of emails.
func contactEmailsString(emails []models.EmailAddressable) string {
	parts := make([]string, 0, len(emails))
	for _, e := range emails {
		if e.GetAddress() != nil {
			parts = append(parts, deref(e.GetAddress()))
		}
	}
	return strings.Join(parts, ", ")
}

// formatContactJSON builds a JSON-serialisable map from a Contact.
func formatContactJSON(c models.Contactable) map[string]interface{} {
	item := map[string]interface{}{
		"id":           deref(c.GetId()),
		"display_name": deref(c.GetDisplayName()),
	}

	if c.GetGivenName() != nil {
		item["given_name"] = deref(c.GetGivenName())
	}
	if c.GetSurname() != nil {
		item["surname"] = deref(c.GetSurname())
	}

	emails := make([]map[string]string, 0)
	for _, e := range c.GetEmailAddresses() {
		em := map[string]string{}
		if e.GetAddress() != nil {
			em["address"] = deref(e.GetAddress())
		}
		if e.GetName() != nil {
			em["name"] = deref(e.GetName())
		}
		emails = append(emails, em)
	}
	item["email_addresses"] = emails

	if c.GetCompanyName() != nil {
		item["company_name"] = deref(c.GetCompanyName())
	}
	if c.GetJobTitle() != nil {
		item["job_title"] = deref(c.GetJobTitle())
	}
	if c.GetDepartment() != nil {
		item["department"] = deref(c.GetDepartment())
	}
	if c.GetMobilePhone() != nil {
		item["mobile_phone"] = deref(c.GetMobilePhone())
	}
	if len(c.GetBusinessPhones()) > 0 {
		item["business_phones"] = c.GetBusinessPhones()
	}
	if len(c.GetHomePhones()) > 0 {
		item["home_phones"] = c.GetHomePhones()
	}

	return item
}

// contactSelectFields defines the $select fields for contacts queries.
var contactSelectFields = []string{
	"id", "displayName", "givenName", "surname", "emailAddresses",
	"companyName", "jobTitle", "department", "mobilePhone",
	"businessPhones", "homePhones",
}

// ──────────────────────────────────────────────
//  Parent command
// ──────────────────────────────────────────────

var contactsCmd = &cobra.Command{
	Use:   "contacts",
	Short: "Outlook Contacts — list, get, search",
}

// ══════════════════════════════════════════════
//  CONTACTS LIST
// ══════════════════════════════════════════════

var contactsListMax int32

var contactsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List contacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		reqConfig := &users.ItemContactsRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemContactsRequestBuilderGetQueryParameters{
				Top:     &contactsListMax,
				Orderby: []string{"displayName"},
				Select:  contactSelectFields,
			},
		}

		result, err := client.Me().Contacts().Get(ctx, reqConfig)
		if err != nil {
			return fmt.Errorf("fetching contacts: %w", err)
		}

		contacts := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(contacts))
			for _, c := range contacts {
				items = append(items, formatContactJSON(c))
			}
			return output.JSON(items)

		case output.FormatPlain:
			var rows [][]string
			for _, c := range contacts {
				rows = append(rows, []string{
					deref(c.GetId()),
					deref(c.GetDisplayName()),
					contactEmailsString(c.GetEmailAddresses()),
					deref(c.GetCompanyName()),
				})
			}
			output.Plain(rows)

		default:
			headers := []string{"NAME", "EMAIL", "COMPANY", "TITLE", "ID"}
			var rows [][]string
			for _, c := range contacts {
				email := contactEmailsString(c.GetEmailAddresses())
				company := deref(c.GetCompanyName())
				title := deref(c.GetJobTitle())
				name := deref(c.GetDisplayName())
				if len(name) > 30 {
					name = name[:27] + "..."
				}
				id := deref(c.GetId())
				if len(id) > 20 {
					id = id[:17] + "..."
				}
				rows = append(rows, []string{name, email, company, title, id})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  CONTACTS GET
// ══════════════════════════════════════════════

var contactsGetID string

var contactsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a single contact by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		if contactsGetID == "" {
			return fmt.Errorf("--id is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		c, err := client.Me().Contacts().ByContactId(contactsGetID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching contact: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			item := formatContactJSON(c)
			if c.GetPersonalNotes() != nil {
				item["personal_notes"] = deref(c.GetPersonalNotes())
			}
			if c.GetBusinessAddress() != nil {
				addr := c.GetBusinessAddress()
				item["business_address"] = map[string]string{
					"street":       deref(addr.GetStreet()),
					"city":         deref(addr.GetCity()),
					"state":        deref(addr.GetState()),
					"country":      deref(addr.GetCountryOrRegion()),
					"postal_code":  deref(addr.GetPostalCode()),
				}
			}
			return output.JSON(item)

		default:
			fmt.Printf("Name:     %s\n", deref(c.GetDisplayName()))
			if c.GetGivenName() != nil || c.GetSurname() != nil {
				fmt.Printf("Full:     %s %s\n", deref(c.GetGivenName()), deref(c.GetSurname()))
			}
			if len(c.GetEmailAddresses()) > 0 {
				fmt.Printf("Email:    %s\n", contactEmailsString(c.GetEmailAddresses()))
			}
			if c.GetCompanyName() != nil {
				fmt.Printf("Company:  %s\n", deref(c.GetCompanyName()))
			}
			if c.GetJobTitle() != nil {
				fmt.Printf("Title:    %s\n", deref(c.GetJobTitle()))
			}
			if c.GetMobilePhone() != nil {
				fmt.Printf("Mobile:   %s\n", deref(c.GetMobilePhone()))
			}
			if len(c.GetBusinessPhones()) > 0 {
				fmt.Printf("Work:     %s\n", strings.Join(c.GetBusinessPhones(), ", "))
			}
			fmt.Printf("ID:       %s\n", deref(c.GetId()))
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  CONTACTS SEARCH
// ══════════════════════════════════════════════

var (
	contactsSearchQuery string
	contactsSearchMax   int32
)

var contactsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search contacts by keyword",
	RunE: func(cmd *cobra.Command, args []string) error {
		if contactsSearchQuery == "" {
			return fmt.Errorf("--query is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Use $filter with contains() on displayName — $search is not supported for contacts
		filterStr := fmt.Sprintf("contains(displayName,'%s') or contains(emailAddresses/any(e:e/address),'%s')",
			contactsSearchQuery, contactsSearchQuery)
		reqConfig := &users.ItemContactsRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemContactsRequestBuilderGetQueryParameters{
				Filter: &filterStr,
				Top:    &contactsSearchMax,
				Select: contactSelectFields,
			},
		}

		result, err := client.Me().Contacts().Get(ctx, reqConfig)
		if err != nil {
			// Fall back to simple displayName filter if complex filter fails
			simpleFilter := fmt.Sprintf("contains(displayName,'%s')", contactsSearchQuery)
			reqConfig.QueryParameters.Filter = &simpleFilter
			result, err = client.Me().Contacts().Get(ctx, reqConfig)
			if err != nil {
				return fmt.Errorf("searching contacts: %w", err)
			}
		}

		contacts := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(contacts))
			for _, c := range contacts {
				items = append(items, formatContactJSON(c))
			}
			return output.JSON(map[string]interface{}{
				"query":   contactsSearchQuery,
				"count":   len(items),
				"results": items,
			})

		case output.FormatPlain:
			var rows [][]string
			for _, c := range contacts {
				rows = append(rows, []string{
					deref(c.GetId()),
					deref(c.GetDisplayName()),
					contactEmailsString(c.GetEmailAddresses()),
				})
			}
			output.Plain(rows)

		default:
			output.Info(fmt.Sprintf("Search results for %q (%d found)", contactsSearchQuery, len(contacts)))
			headers := []string{"NAME", "EMAIL", "COMPANY", "ID"}
			var rows [][]string
			for _, c := range contacts {
				name := deref(c.GetDisplayName())
				email := contactEmailsString(c.GetEmailAddresses())
				company := deref(c.GetCompanyName())
				id := deref(c.GetId())
				if len(id) > 20 {
					id = id[:17] + "..."
				}
				rows = append(rows, []string{name, email, company, id})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  Wire up commands + flags
// ══════════════════════════════════════════════

func init() {
	// contacts list
	contactsListCmd.Flags().Int32Var(&contactsListMax, "max", 50, "Maximum contacts to return")

	// contacts get
	contactsGetCmd.Flags().StringVar(&contactsGetID, "id", "", "Contact ID")

	// contacts search
	contactsSearchCmd.Flags().StringVar(&contactsSearchQuery, "query", "", "Search query (name or email)")
	contactsSearchCmd.Flags().Int32Var(&contactsSearchMax, "max", 25, "Maximum results to return")

	// Wire
	contactsCmd.AddCommand(contactsListCmd)
	contactsCmd.AddCommand(contactsGetCmd)
	contactsCmd.AddCommand(contactsSearchCmd)
}

