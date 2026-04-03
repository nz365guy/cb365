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
//  Calendar helpers
// ──────────────────────────────────────────────

// parseRFC3339Strict parses a datetime string and rejects bare datetimes without timezone.
// Safety rule #1: all datetimes MUST include a timezone offset.
func parseRFC3339Strict(s string) (time.Time, error) {
	// Reject bare datetimes like "2026-04-10T09:00:00" (no offset)
	if !strings.Contains(s, "Z") && !strings.Contains(s, "+") && !strings.ContainsAny(s[len(s)-6:], "+-") {
		return time.Time{}, fmt.Errorf("datetime %q missing timezone offset — use full RFC3339 format (e.g. 2026-04-10T09:00:00+12:00)", s)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid RFC3339 datetime %q: %w", s, err)
	}
	return t, nil
}

// rejectPastEvent enforces safety rule #2: no modifications to past events.
func rejectPastEvent(startTime time.Time, action string) error {
	if startTime.Before(time.Now()) {
		return fmt.Errorf("cannot %s event starting at %s — it is in the past (past events are historical records)", action, startTime.Format(time.RFC3339))
	}
	return nil
}

// formatEventJSON builds a JSON-serialisable map from an Event.
func formatEventJSON(evt models.Eventable) map[string]interface{} {
	item := map[string]interface{}{
		"id":      deref(evt.GetId()),
		"subject": deref(evt.GetSubject()),
	}

	if evt.GetStart() != nil {
		item["start"] = map[string]string{
			"dateTime": deref(evt.GetStart().GetDateTime()),
			"timeZone": deref(evt.GetStart().GetTimeZone()),
		}
	}
	if evt.GetEnd() != nil {
		item["end"] = map[string]string{
			"dateTime": deref(evt.GetEnd().GetDateTime()),
			"timeZone": deref(evt.GetEnd().GetTimeZone()),
		}
	}

	if evt.GetOrganizer() != nil && evt.GetOrganizer().GetEmailAddress() != nil {
		item["organizer"] = map[string]string{
			"name":    deref(evt.GetOrganizer().GetEmailAddress().GetName()),
			"address": deref(evt.GetOrganizer().GetEmailAddress().GetAddress()),
		}
	}

	attendees := make([]map[string]interface{}, 0)
	for _, a := range evt.GetAttendees() {
		if a.GetEmailAddress() != nil {
			att := map[string]interface{}{
				"name":    deref(a.GetEmailAddress().GetName()),
				"address": deref(a.GetEmailAddress().GetAddress()),
			}
			if a.GetTypeEscaped() != nil {
				att["type"] = a.GetTypeEscaped().String()
			}
			attendees = append(attendees, att)
		}
	}
	item["attendees"] = attendees

	if evt.GetLocation() != nil {
		item["location"] = deref(evt.GetLocation().GetDisplayName())
	}
	if evt.GetIsOnlineMeeting() != nil {
		item["is_online_meeting"] = *evt.GetIsOnlineMeeting()
	}
	if evt.GetOnlineMeetingUrl() != nil {
		item["online_meeting_url"] = deref(evt.GetOnlineMeetingUrl())
	}
	if evt.GetIsAllDay() != nil {
		item["is_all_day"] = *evt.GetIsAllDay()
	}
	if evt.GetIsCancelled() != nil {
		item["is_cancelled"] = *evt.GetIsCancelled()
	}
	if evt.GetShowAs() != nil {
		item["show_as"] = evt.GetShowAs().String()
	}
	if evt.GetSeriesMasterId() != nil {
		item["series_master_id"] = deref(evt.GetSeriesMasterId())
	}
	if evt.GetRecurrence() != nil {
		item["is_recurring"] = true
	}
	item["body_preview"] = deref(evt.GetBodyPreview())

	return item
}

// eventTimeString formats event start/end for human display.
func eventTimeString(dtz models.DateTimeTimeZoneable) string {
	if dtz == nil {
		return ""
	}
	dt := deref(dtz.GetDateTime())
	tz := deref(dtz.GetTimeZone())
	// Try to parse and format nicely
	t, err := time.Parse("2006-01-02T15:04:05.0000000", dt)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", dt)
	}
	if err != nil {
		return dt // fallback to raw
	}
	if tz == "Pacific/Auckland" || tz == "New Zealand Standard Time" {
		return t.Format("2 Jan 3:04pm")
	}
	return t.Format("2 Jan 3:04pm") + " (" + tz + ")"
}

// ──────────────────────────────────────────────
//  Parent command
// ──────────────────────────────────────────────

var calendarCmd = &cobra.Command{
	Use:   "calendar",
	Short: "Outlook Calendar — list, create, update, delete events",
}

// ══════════════════════════════════════════════
//  CALENDAR LIST
// ══════════════════════════════════════════════

var (
	calListFrom string
	calListTo   string
)

var calListCmd = &cobra.Command{
	Use:   "list",
	Short: "List calendar events in a date range",
	RunE: func(cmd *cobra.Command, args []string) error {
		if calListFrom == "" || calListTo == "" {
			return fmt.Errorf("--from and --to are required (ISO date or RFC3339)")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		reqConfig := &users.ItemCalendarViewRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemCalendarViewRequestBuilderGetQueryParameters{
				StartDateTime: &calListFrom,
				EndDateTime:   &calListTo,
				Select:        []string{"id", "subject", "start", "end", "organizer", "attendees", "location", "isOnlineMeeting", "onlineMeetingUrl", "isAllDay", "isCancelled", "showAs", "seriesMasterId", "recurrence", "bodyPreview"},
			},
		}

		result, err := client.Me().CalendarView().Get(ctx, reqConfig)
		if err != nil {
			return fmt.Errorf("fetching calendar events: %w", err)
		}

		events := result.GetValue()
		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			items := make([]map[string]interface{}, 0, len(events))
			for _, evt := range events {
				items = append(items, formatEventJSON(evt))
			}
			return output.JSON(items)

		case output.FormatPlain:
			var rows [][]string
			for _, evt := range events {
				start := ""
				if evt.GetStart() != nil {
					start = deref(evt.GetStart().GetDateTime())
				}
				rows = append(rows, []string{
					deref(evt.GetId()),
					start,
					deref(evt.GetSubject()),
				})
			}
			output.Plain(rows)

		default:
			headers := []string{"DATE", "TIME", "SUBJECT", "STATUS", "ID"}
			var rows [][]string
			for _, evt := range events {
				start := eventTimeString(evt.GetStart())
				end := eventTimeString(evt.GetEnd())
				timeStr := start
				if end != "" {
					timeStr = start + " → " + end
				}
				// Extract date from start
				dateStr := ""
				if evt.GetStart() != nil {
					dt := deref(evt.GetStart().GetDateTime())
					t, err := time.Parse("2006-01-02T15:04:05.0000000", dt)
					if err != nil {
						t, _ = time.Parse("2006-01-02T15:04:05", dt)
					}
					if !t.IsZero() {
						dateStr = t.Format("Mon 2 Jan")
					}
				}
				showAs := ""
				if evt.GetShowAs() != nil {
					showAs = evt.GetShowAs().String()
				}
				subject := deref(evt.GetSubject())
				if len(subject) > 50 {
					subject = subject[:47] + "..."
				}
				id := deref(evt.GetId())
				if len(id) > 20 {
					id = id[:17] + "..."
				}
				_ = timeStr // use date + just start time for table
				rows = append(rows, []string{dateStr, start, subject, showAs, id})
			}
			output.Table(headers, rows)
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  CALENDAR GET
// ══════════════════════════════════════════════

var calGetID string

var calGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a single calendar event by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		if calGetID == "" {
			return fmt.Errorf("--id is required")
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		evt, err := client.Me().Events().ByEventId(calGetID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching event: %w", err)
		}

		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			item := formatEventJSON(evt)
			if evt.GetBody() != nil {
				item["body"] = deref(evt.GetBody().GetContent())
				item["body_type"] = evt.GetBody().GetContentType().String()
			}
			if evt.GetWebLink() != nil {
				item["web_link"] = deref(evt.GetWebLink())
			}
			return output.JSON(item)

		default:
			fmt.Printf("Subject:   %s\n", deref(evt.GetSubject()))
			fmt.Printf("Start:     %s\n", eventTimeString(evt.GetStart()))
			fmt.Printf("End:       %s\n", eventTimeString(evt.GetEnd()))
			if evt.GetLocation() != nil && deref(evt.GetLocation().GetDisplayName()) != "" {
				fmt.Printf("Location:  %s\n", deref(evt.GetLocation().GetDisplayName()))
			}
			if evt.GetIsOnlineMeeting() != nil && *evt.GetIsOnlineMeeting() {
				fmt.Printf("Online:    Yes")
				if evt.GetOnlineMeetingUrl() != nil {
					fmt.Printf(" (%s)", deref(evt.GetOnlineMeetingUrl()))
				}
				fmt.Println()
			}
			if len(evt.GetAttendees()) > 0 {
				parts := make([]string, 0)
				for _, a := range evt.GetAttendees() {
					if a.GetEmailAddress() != nil {
						parts = append(parts, recipientString(a))
					}
				}
				fmt.Printf("Attendees: %s\n", strings.Join(parts, ", "))
			}
			if evt.GetShowAs() != nil {
				fmt.Printf("Show as:   %s\n", evt.GetShowAs().String())
			}
			if evt.GetSeriesMasterId() != nil {
				fmt.Printf("Series:    Instance of %s\n", deref(evt.GetSeriesMasterId()))
			}
			fmt.Printf("ID:        %s\n", deref(evt.GetId()))
			if evt.GetBody() != nil && deref(evt.GetBody().GetContent()) != "" {
				fmt.Println()
				fmt.Println("─── Body ───")
				fmt.Println(deref(evt.GetBody().GetContent()))
			}
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  CALENDAR CREATE
// ══════════════════════════════════════════════

var (
	calCreateSubject  string
	calCreateStart    string
	calCreateEnd      string
	calCreateAttendee []string
	calCreateTeams    bool
	calCreateBody     string
)

var calCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a calendar event",
	RunE: func(cmd *cobra.Command, args []string) error {
		if calCreateSubject == "" || calCreateStart == "" || calCreateEnd == "" {
			return fmt.Errorf("--subject, --start, and --end are required")
		}

		// Safety rule #1: timezone validation
		startTime, err := parseRFC3339Strict(calCreateStart)
		if err != nil {
			return err
		}
		endTime, err := parseRFC3339Strict(calCreateEnd)
		if err != nil {
			return err
		}

		// Safety rule #2: no past events
		if err := rejectPastEvent(startTime, "create"); err != nil {
			return err
		}

		// Validate end is after start
		if !endTime.After(startTime) {
			return fmt.Errorf("--end must be after --start")
		}

		format := output.Resolve(flagJSON, flagPlain)

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would create event %q on %s", calCreateSubject, startTime.Format("2 Jan 2006 3:04pm")))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "create_event",
					"subject": calCreateSubject, "start": calCreateStart, "end": calCreateEnd,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Safety rule #3: duplicate detection
		dayStart := startTime.Format("2006-01-02") + "T00:00:00" + startTime.Format("-07:00")
		dayEnd := startTime.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00" + startTime.Format("-07:00")
		viewConfig := &users.ItemCalendarViewRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemCalendarViewRequestBuilderGetQueryParameters{
				StartDateTime: &dayStart,
				EndDateTime:   &dayEnd,
				Select:        []string{"id", "subject", "start", "end"},
			},
		}
		existingResult, err := client.Me().CalendarView().Get(ctx, viewConfig)
		if err != nil {
			return fmt.Errorf("checking for duplicates: %w", err)
		}
		for _, existing := range existingResult.GetValue() {
			if strings.EqualFold(deref(existing.GetSubject()), calCreateSubject) {
				return fmt.Errorf("duplicate detected: event %q already exists on %s (ID: %s) — use --force to override or choose a different time",
					deref(existing.GetSubject()), startTime.Format("2 Jan 2006"), deref(existing.GetId()))
			}
		}

		// Build event
		evt := models.NewEvent()
		evt.SetSubject(ptr(calCreateSubject))

		start := models.NewDateTimeTimeZone()
		start.SetDateTime(ptr(startTime.Format("2006-01-02T15:04:05")))
		start.SetTimeZone(ptr(startTime.Location().String()))
		evt.SetStart(start)

		end := models.NewDateTimeTimeZone()
		end.SetDateTime(ptr(endTime.Format("2006-01-02T15:04:05")))
		end.SetTimeZone(ptr(endTime.Location().String()))
		evt.SetEnd(end)

		if calCreateBody != "" {
			body := models.NewItemBody()
			body.SetContent(ptr(calCreateBody))
			ct := models.TEXT_BODYTYPE
			body.SetContentType(&ct)
			evt.SetBody(body)
		}

		if calCreateTeams {
			isOnline := true
			evt.SetIsOnlineMeeting(&isOnline)
			provider := models.TEAMSFORBUSINESS_ONLINEMEETINGPROVIDERTYPE
			evt.SetOnlineMeetingProvider(&provider)
		}

		if len(calCreateAttendee) > 0 {
			attendees := make([]models.Attendeeable, 0, len(calCreateAttendee))
			for _, email := range calCreateAttendee {
				a := models.NewAttendee()
				addr := models.NewEmailAddress()
				addr.SetAddress(ptr(strings.TrimSpace(email)))
				a.SetEmailAddress(addr)
				at := models.REQUIRED_ATTENDEETYPE
				a.SetTypeEscaped(&at)
				attendees = append(attendees, a)
			}
			evt.SetAttendees(attendees)
		}

		result, err := client.Me().Events().Post(ctx, evt, nil)
		if err != nil {
			return fmt.Errorf("creating event: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(formatEventJSON(result))
		default:
			output.Success(fmt.Sprintf("Created event %q (ID: %s)", deref(result.GetSubject()), deref(result.GetId())))
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  CALENDAR UPDATE
// ══════════════════════════════════════════════

var (
	calUpdateID      string
	calUpdateSubject string
	calUpdateStart   string
	calUpdateEnd     string
)

var calUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a calendar event",
	RunE: func(cmd *cobra.Command, args []string) error {
		if calUpdateID == "" {
			return fmt.Errorf("--id is required")
		}

		format := output.Resolve(flagJSON, flagPlain)

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would update event %s", calUpdateID))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "update_event", "id": calUpdateID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Fetch existing event to enforce safety rules
		existing, err := client.Me().Events().ByEventId(calUpdateID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching event for safety check: %w", err)
		}

		// Safety rule #4: reject series master modification
		if existing.GetRecurrence() != nil && existing.GetSeriesMasterId() == nil {
			return fmt.Errorf("cannot update recurring event series master — modify specific future instances instead (use calendar list to find the instance ID)")
		}

		// Safety rule #2: check existing event is not in the past
		if existing.GetStart() != nil {
			dt := deref(existing.GetStart().GetDateTime())
			tz := deref(existing.GetStart().GetTimeZone())
			loc, locErr := time.LoadLocation(tz)
			if locErr != nil {
				loc = time.UTC
			}
			existingStart, parseErr := time.ParseInLocation("2006-01-02T15:04:05.0000000", dt, loc)
			if parseErr != nil {
				existingStart, _ = time.ParseInLocation("2006-01-02T15:04:05", dt, loc)
			}
			if !existingStart.IsZero() {
				if err := rejectPastEvent(existingStart, "update"); err != nil {
					return err
				}
			}
		}

		// Build update patch
		patch := models.NewEvent()
		hasUpdate := false

		if calUpdateSubject != "" {
			patch.SetSubject(ptr(calUpdateSubject))
			hasUpdate = true
		}
		if calUpdateStart != "" {
			startTime, err := parseRFC3339Strict(calUpdateStart)
			if err != nil {
				return err
			}
			if err := rejectPastEvent(startTime, "update (new start time)"); err != nil {
				return err
			}
			s := models.NewDateTimeTimeZone()
			s.SetDateTime(ptr(startTime.Format("2006-01-02T15:04:05")))
			s.SetTimeZone(ptr(startTime.Location().String()))
			patch.SetStart(s)
			hasUpdate = true
		}
		if calUpdateEnd != "" {
			endTime, err := parseRFC3339Strict(calUpdateEnd)
			if err != nil {
				return err
			}
			e := models.NewDateTimeTimeZone()
			e.SetDateTime(ptr(endTime.Format("2006-01-02T15:04:05")))
			e.SetTimeZone(ptr(endTime.Location().String()))
			patch.SetEnd(e)
			hasUpdate = true
		}

		if !hasUpdate {
			return fmt.Errorf("nothing to update — specify --subject, --start, or --end")
		}

		result, err := client.Me().Events().ByEventId(calUpdateID).Patch(ctx, patch, nil)
		if err != nil {
			return fmt.Errorf("updating event: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(formatEventJSON(result))
		default:
			output.Success(fmt.Sprintf("Updated event %q", deref(result.GetSubject())))
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  CALENDAR DELETE
// ══════════════════════════════════════════════

var (
	calDeleteID    string
	calDeleteForce bool
)

var calDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a calendar event",
	RunE: func(cmd *cobra.Command, args []string) error {
		if calDeleteID == "" {
			return fmt.Errorf("--id is required")
		}
		if !calDeleteForce {
			return fmt.Errorf("--force is required to confirm deletion")
		}

		format := output.Resolve(flagJSON, flagPlain)

		if flagDryRun {
			output.Info(fmt.Sprintf("[dry-run] Would delete event %s", calDeleteID))
			if format == output.FormatJSON {
				return output.JSON(map[string]interface{}{
					"dry_run": true, "action": "delete_event", "id": calDeleteID,
				})
			}
			return nil
		}

		client, err := newGraphClient()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Fetch event for safety checks before deleting
		existing, err := client.Me().Events().ByEventId(calDeleteID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching event for safety check: %w", err)
		}

		// Safety rule #4: reject series master deletion
		if existing.GetRecurrence() != nil && existing.GetSeriesMasterId() == nil {
			return fmt.Errorf("cannot delete recurring event series master — delete specific future instances instead")
		}

		// Safety rule #2: no past event deletion
		if existing.GetStart() != nil {
			dt := deref(existing.GetStart().GetDateTime())
			tz := deref(existing.GetStart().GetTimeZone())
			loc, locErr := time.LoadLocation(tz)
			if locErr != nil {
				loc = time.UTC
			}
			existingStart, parseErr := time.ParseInLocation("2006-01-02T15:04:05.0000000", dt, loc)
			if parseErr != nil {
				existingStart, _ = time.ParseInLocation("2006-01-02T15:04:05", dt, loc)
			}
			if !existingStart.IsZero() {
				if err := rejectPastEvent(existingStart, "delete"); err != nil {
					return err
				}
			}
		}

		if err := client.Me().Events().ByEventId(calDeleteID).Delete(ctx, nil); err != nil {
			return fmt.Errorf("deleting event: %w", err)
		}

		switch format {
		case output.FormatJSON:
			return output.JSON(map[string]string{"deleted": calDeleteID, "status": "ok"})
		default:
			output.Success("Deleted event")
		}
		return nil
	},
}

// ══════════════════════════════════════════════
//  Wire up commands + flags
// ══════════════════════════════════════════════

func init() {
	// calendar list
	calListCmd.Flags().StringVar(&calListFrom, "from", "", "Start date (ISO 8601 or RFC3339)")
	calListCmd.Flags().StringVar(&calListTo, "to", "", "End date (ISO 8601 or RFC3339)")

	// calendar get
	calGetCmd.Flags().StringVar(&calGetID, "id", "", "Event ID")

	// calendar create
	calCreateCmd.Flags().StringVar(&calCreateSubject, "subject", "", "Event subject/title")
	calCreateCmd.Flags().StringVar(&calCreateStart, "start", "", "Start time (RFC3339 with timezone, e.g. 2026-04-10T09:00:00+12:00)")
	calCreateCmd.Flags().StringVar(&calCreateEnd, "end", "", "End time (RFC3339 with timezone)")
	calCreateCmd.Flags().StringArrayVar(&calCreateAttendee, "attendee", nil, "Attendee email (repeatable)")
	calCreateCmd.Flags().BoolVar(&calCreateTeams, "teams", false, "Add Microsoft Teams meeting link")
	calCreateCmd.Flags().StringVar(&calCreateBody, "body", "", "Event body/description (plain text)")

	// calendar update
	calUpdateCmd.Flags().StringVar(&calUpdateID, "id", "", "Event ID to update")
	calUpdateCmd.Flags().StringVar(&calUpdateSubject, "subject", "", "New subject")
	calUpdateCmd.Flags().StringVar(&calUpdateStart, "start", "", "New start time (RFC3339 with timezone)")
	calUpdateCmd.Flags().StringVar(&calUpdateEnd, "end", "", "New end time (RFC3339 with timezone)")

	// calendar delete
	calDeleteCmd.Flags().StringVar(&calDeleteID, "id", "", "Event ID to delete")
	calDeleteCmd.Flags().BoolVar(&calDeleteForce, "force", false, "Confirm destructive operation")

	// Wire
	calendarCmd.AddCommand(calListCmd)
	calendarCmd.AddCommand(calGetCmd)
	calendarCmd.AddCommand(calCreateCmd)
	calendarCmd.AddCommand(calUpdateCmd)
	calendarCmd.AddCommand(calDeleteCmd)
}

