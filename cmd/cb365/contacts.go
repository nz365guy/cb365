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
		// Safety: warn on large exports
		if contactsListMax > 100 {
			output.Info(fmt.Sprintf("Warning: requesting %d contacts — large exports should be reviewed before sharing externally", contactsListMax))
		}

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

var (
	contactsGetID             string
	contactsGetIncludePrivate bool
)

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
			if contactsGetIncludePrivate {
				// Private fields: only included with --include-private
				if c.GetPersonalNotes() != nil {
					item["personal_notes"] = deref(c.GetPersonalNotes())
				}
				if c.GetHomeAddress() != nil {
					addr := c.GetHomeAddress()
					item["home_address"] = map[string]string{
						"street":      deref(addr.GetStreet()),
						"city":        deref(addr.GetCity()),
						"state":       deref(addr.GetState()),
						"country":     deref(addr.GetCountryOrRegion()),
						"postal_code": deref(addr.GetPostalCode()),
					}
				}
				if len(c.GetHomePhones()) > 0 {
					item["home_phones"] = c.GetHomePhones()
				}
			} else {
				if c.GetPersonalNotes() != nil {
					item["personal_notes"] = "[REDACTED — use --include-private]"
				}
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


// ──────────────────────────────────────────────
//  contacts create
// ──────────────────────────────────────────────

var contactsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new contact",
	Long: `Create a new Outlook contact.

Examples:
  cb365 contacts create --given-name "Jane" --surname "Doe" --email "jane@example.com"
  cb365 contacts create --given-name "John" --surname "Smith" --email "john@acme.com" --company "Acme Corp" --job-title "CEO" --phone "+64 21 555 1234"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		givenName, _ := cmd.Flags().GetString("given-name")
		surname, _ := cmd.Flags().GetString("surname")
		email, _ := cmd.Flags().GetString("email")
		company, _ := cmd.Flags().GetString("company")
		jobTitle, _ := cmd.Flags().GetString("job-title")
		phone, _ := cmd.Flags().GetString("phone")
		department, _ := cmd.Flags().GetString("department")
		notes, _ := cmd.Flags().GetString("notes")

		if givenName == "" && surname == "" {
			return fmt.Errorf("at least --given-name or --surname is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if flagDryRun {
			displayName := strings.TrimSpace(givenName + " " + surname)
			output.Info(fmt.Sprintf("[DRY RUN] Would create contact: %s", displayName))
			return nil
		}

		contact := models.NewContact()
		if givenName != "" {
			contact.SetGivenName(&givenName)
		}
		if surname != "" {
			contact.SetSurname(&surname)
		}
		if email != "" {
			emailAddr := models.NewEmailAddress()
			emailAddr.SetAddress(&email)
			emailAddr.SetName(ptr(strings.TrimSpace(givenName + " " + surname)))
			contact.SetEmailAddresses([]models.EmailAddressable{emailAddr})
		}
		if company != "" {
			contact.SetCompanyName(&company)
		}
		if jobTitle != "" {
			contact.SetJobTitle(&jobTitle)
		}
		if phone != "" {
			contact.SetBusinessPhones([]string{phone})
		}
		if department != "" {
			contact.SetDepartment(&department)
		}
		if notes != "" {
			body := models.NewItemBody()
			contentType := models.TEXT_BODYTYPE
			body.SetContentType(&contentType)
			body.SetContent(&notes)
			contact.SetPersonalNotes(&notes)
		}

		created, err := client.Me().Contacts().Post(ctx, contact, nil)
		if err != nil {
			return fmt.Errorf("creating contact: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(formatContactJSON(created))
		default:
			output.Success(fmt.Sprintf("Created contact: %s (id: %s)", deref(created.GetDisplayName()), deref(created.GetId())))
		}
		return nil
	},
}

// ──────────────────────────────────────────────
//  contacts update
// ──────────────────────────────────────────────

var contactsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing contact",
	Long: `Update fields on an existing Outlook contact.

Examples:
  cb365 contacts update --id CONTACT_ID --company "New Corp"
  cb365 contacts update --id CONTACT_ID --job-title "CTO" --phone "+64 21 555 9999"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		idFlag, _ := cmd.Flags().GetString("id")
		givenName, _ := cmd.Flags().GetString("given-name")
		surname, _ := cmd.Flags().GetString("surname")
		email, _ := cmd.Flags().GetString("email")
		company, _ := cmd.Flags().GetString("company")
		jobTitle, _ := cmd.Flags().GetString("job-title")
		phone, _ := cmd.Flags().GetString("phone")
		department, _ := cmd.Flags().GetString("department")

		if idFlag == "" {
			return fmt.Errorf("--id is required")
		}

		hasUpdate := givenName != "" || surname != "" || email != "" || company != "" || jobTitle != "" || phone != "" || department != ""
		if !hasUpdate {
			return fmt.Errorf("at least one field to update is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if flagDryRun {
			output.Info(fmt.Sprintf("[DRY RUN] Would update contact %s", idFlag))
			return nil
		}

		contact := models.NewContact()
		if givenName != "" {
			contact.SetGivenName(&givenName)
		}
		if surname != "" {
			contact.SetSurname(&surname)
		}
		if email != "" {
			emailAddr := models.NewEmailAddress()
			emailAddr.SetAddress(&email)
			contact.SetEmailAddresses([]models.EmailAddressable{emailAddr})
		}
		if company != "" {
			contact.SetCompanyName(&company)
		}
		if jobTitle != "" {
			contact.SetJobTitle(&jobTitle)
		}
		if phone != "" {
			contact.SetBusinessPhones([]string{phone})
		}
		if department != "" {
			contact.SetDepartment(&department)
		}

		updated, err := client.Me().Contacts().ByContactId(idFlag).Patch(ctx, contact, nil)
		if err != nil {
			return fmt.Errorf("updating contact: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)
		switch format {
		case output.FormatJSON:
			return output.JSON(formatContactJSON(updated))
		default:
			output.Success(fmt.Sprintf("Updated contact: %s", deref(updated.GetDisplayName())))
		}
		return nil
	},
}

func init() {
	// contacts list
	contactsListCmd.Flags().Int32Var(&contactsListMax, "max", 50, "Maximum contacts to return")

	// contacts get
	contactsGetCmd.Flags().StringVar(&contactsGetID, "id", "", "Contact ID")
	contactsGetCmd.Flags().BoolVar(&contactsGetIncludePrivate, "include-private", false, "Include private fields (notes, home address, home phone)")

	// contacts search
	contactsSearchCmd.Flags().StringVar(&contactsSearchQuery, "query", "", "Search query (name or email)")
	contactsSearchCmd.Flags().Int32Var(&contactsSearchMax, "max", 25, "Maximum results to return")

	// Wire
	contactsCmd.AddCommand(contactsListCmd)
	contactsCmd.AddCommand(contactsGetCmd)
	contactsCmd.AddCommand(contactsSearchCmd)

	// contacts create
	contactsCreateCmd.Flags().String("given-name", "", "First name")
	contactsCreateCmd.Flags().String("surname", "", "Last name")
	contactsCreateCmd.Flags().String("email", "", "Email address")
	contactsCreateCmd.Flags().String("company", "", "Company name")
	contactsCreateCmd.Flags().String("job-title", "", "Job title")
	contactsCreateCmd.Flags().String("phone", "", "Business phone")
	contactsCreateCmd.Flags().String("department", "", "Department")
	contactsCreateCmd.Flags().String("notes", "", "Personal notes")
	contactsCmd.AddCommand(contactsCreateCmd)

	// contacts update
	contactsUpdateCmd.Flags().String("id", "", "Contact ID (required)")
	contactsUpdateCmd.Flags().String("given-name", "", "First name")
	contactsUpdateCmd.Flags().String("surname", "", "Last name")
	contactsUpdateCmd.Flags().String("email", "", "Email address")
	contactsUpdateCmd.Flags().String("company", "", "Company name")
	contactsUpdateCmd.Flags().String("job-title", "", "Job title")
	contactsUpdateCmd.Flags().String("phone", "", "Business phone")
	contactsUpdateCmd.Flags().String("department", "", "Department")
	contactsCmd.AddCommand(contactsUpdateCmd)
}


