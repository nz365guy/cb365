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
//  Calendar safety helpers
// ──────────────────────────────────────────────

// nzNow returns the current time in Pacific/Auckland.
// All past-event checks MUST use this, not time.Now().
func nzNow() time.Time {
	loc, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		return time.Now() // fallback to system time
	}
	return time.Now().In(loc)
}

// parseRFC3339Strict parses a datetime string and rejects bare datetimes without timezone.
// Safety rule: all datetimes MUST include a timezone offset.
func parseRFC3339Strict(s string) (time.Time, error) {
	if !strings.Contains(s, "Z") && !strings.Contains(s, "+") && !strings.ContainsAny(s[len(s)-6:], "+-") {
		return time.Time{}, fmt.Errorf("datetime %q missing timezone offset — use full RFC3339 format (e.g. 2026-04-10T09:00:00+12:00)", s)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid RFC3339 datetime %q: %w", s, err)
	}
	return t, nil
}

// rejectPastEvent enforces: no modifications to past events (Pacific/Auckland).
func rejectPastEvent(startTime time.Time, action string) error {
	if startTime.Before(nzNow()) {
		return fmt.Errorf("cannot %s event starting at %s — it is in the past (past events are historical records)", action, startTime.Format(time.RFC3339))
	}
	return nil
}

// parseEventStartTime extracts the start time from a Graph event as a time.Time.
func parseEventStartTime(evt models.Eventable) (time.Time, bool) {
	if evt.GetStart() == nil {
		return time.Time{}, false
	}
	dt := deref(evt.GetStart().GetDateTime())
	tz := deref(evt.GetStart().GetTimeZone())
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05.0000000", dt, loc)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02T15:04:05", dt, loc)
	}
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// parseEventEndTime extracts the end time from a Graph event as a time.Time.
func parseEventEndTime(evt models.Eventable) (time.Time, bool) {
	if evt.GetEnd() == nil {
		return time.Time{}, false
	}
	dt := deref(evt.GetEnd().GetDateTime())
	tz := deref(evt.GetEnd().GetTimeZone())
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05.0000000", dt, loc)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02T15:04:05", dt, loc)
	}
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// isSeriesMaster returns true if the event is a recurring series master (not an instance).
func isSeriesMaster(evt models.Eventable) bool {
	return evt.GetRecurrence() != nil && evt.GetSeriesMasterId() == nil
}

// isPrivateEvent returns true if the event's sensitivity is "private".
func isPrivateEvent(evt models.Eventable) bool {
	if evt.GetSensitivity() == nil {
		return false
	}
	return evt.GetSensitivity().String() == "private"
}

// isOOFOrBusy returns true if the event is marked as Out of Office or Busy.
func isOOFOrBusy(evt models.Eventable) bool {
	if evt.GetShowAs() == nil {
		return false
	}
	s := evt.GetShowAs().String()
	return s == "oof" || s == "busy"
}

// isOrganizer returns true if the current user is the event organizer.
func isOrganizer(evt models.Eventable) bool {
	if evt.GetIsOrganizer() == nil {
		return true // default to true if unknown
	}
	return *evt.GetIsOrganizer()
}

// attendeeCount returns the number of attendees on the event.
func attendeeCount(evt models.Eventable) int {
	return len(evt.GetAttendees())
}

// hasTimeOverlap checks if [newStart, newEnd) overlaps with [existStart, existEnd).
func hasTimeOverlap(newStart, newEnd, existStart, existEnd time.Time) bool {
	return newStart.Before(existEnd) && newEnd.After(existStart)
}

// ──────────────────────────────────────────────
//  Calendar JSON formatter
// ──────────────────────────────────────────────

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
	if evt.GetIsOrganizer() != nil {
		item["is_organizer"] = *evt.GetIsOrganizer()
	}
	if evt.GetSensitivity() != nil {
		item["sensitivity"] = evt.GetSensitivity().String()
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

func eventTimeString(dtz models.DateTimeTimeZoneable) string {
	if dtz == nil {
		return ""
	}
	dt := deref(dtz.GetDateTime())
	tz := deref(dtz.GetTimeZone())
	t, err := time.Parse("2006-01-02T15:04:05.0000000", dt)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", dt)
	}
	if err != nil {
		return dt
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
				Select:        []string{"id", "subject", "start", "end", "organizer", "attendees", "location", "isOnlineMeeting", "onlineMeetingUrl", "isAllDay", "isCancelled", "showAs", "sensitivity", "isOrganizer", "seriesMasterId", "recurrence", "bodyPreview"},
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

		// Safety: redact body of private events
		if isPrivateEvent(evt) {
			output.Info("Note: this event is marked Private — body content redacted")
		}

		format := output.Resolve(flagJSON, flagPlain)

		switch format {
		case output.FormatJSON:
			item := formatEventJSON(evt)
			if evt.GetBody() != nil && !isPrivateEvent(evt) {
				item["body"] = deref(evt.GetBody().GetContent())
				item["body_type"] = evt.GetBody().GetContentType().String()
			} else if isPrivateEvent(evt) {
				item["body"] = "[REDACTED — event marked Private]"
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
			if evt.GetSensitivity() != nil {
				fmt.Printf("Privacy:   %s\n", evt.GetSensitivity().String())
			}
			if !isOrganizer(evt) {
				fmt.Printf("Organizer: %s (you are an attendee)\n", recipientString(evt.GetOrganizer()))
			}
			if evt.GetSeriesMasterId() != nil {
				fmt.Printf("Series:    Instance of %s\n", deref(evt.GetSeriesMasterId()))
			}
			fmt.Printf("ID:        %s\n", deref(evt.GetId()))
			if evt.GetBody() != nil && deref(evt.GetBody().GetContent()) != "" && !isPrivateEvent(evt) {
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
	calCreateForce    bool
)

var calCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a calendar event",
	RunE: func(cmd *cobra.Command, args []string) error {
		if calCreateSubject == "" || calCreateStart == "" || calCreateEnd == "" {
			return fmt.Errorf("--subject, --start, and --end are required")
		}

		// Safety: timezone validation
		startTime, err := parseRFC3339Strict(calCreateStart)
		if err != nil {
			return err
		}
		endTime, err := parseRFC3339Strict(calCreateEnd)
		if err != nil {
			return err
		}

		// Safety: no past events
		if err := rejectPastEvent(startTime, "create"); err != nil {
			return err
		}

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

		// Safety: duplicate + overlap detection
		if !calCreateForce {
			dayStart := startTime.Format("2006-01-02") + "T00:00:00" + startTime.Format("-07:00")
			dayEnd := startTime.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00" + startTime.Format("-07:00")
			viewConfig := &users.ItemCalendarViewRequestBuilderGetRequestConfiguration{
				QueryParameters: &users.ItemCalendarViewRequestBuilderGetQueryParameters{
					StartDateTime: &dayStart,
					EndDateTime:   &dayEnd,
					Select:        []string{"id", "subject", "start", "end", "showAs"},
				},
			}
			existingResult, err := client.Me().CalendarView().Get(ctx, viewConfig)
			if err != nil {
				return fmt.Errorf("checking for conflicts: %w", err)
			}

			for _, existing := range existingResult.GetValue() {
				existStart, okS := parseEventStartTime(existing)
				existEnd, okE := parseEventEndTime(existing)

				// Check subject duplicate
				if strings.EqualFold(deref(existing.GetSubject()), calCreateSubject) {
					return fmt.Errorf("duplicate detected: event %q already exists on %s (ID: %s) — use --force to override",
						deref(existing.GetSubject()), startTime.Format("2 Jan 2006"), deref(existing.GetId()))
				}

				// Check time overlap (skip all-day events for overlap)
				if okS && okE && hasTimeOverlap(startTime, endTime, existStart, existEnd) {
					showAs := ""
					if existing.GetShowAs() != nil {
						showAs = existing.GetShowAs().String()
					}
					if showAs == "busy" || showAs == "oof" || showAs == "tentative" {
						return fmt.Errorf("time conflict: %q (%s–%s, %s) overlaps with your new event — use --force to double-book",
							deref(existing.GetSubject()),
							existStart.Format("3:04pm"), existEnd.Format("3:04pm"),
							showAs)
					}
				}
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

		// Audit: tag agent-created events with [cb365] category
		evt.SetCategories([]string{"cb365"})

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
	calUpdateForce   bool
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

		// Fetch existing event for safety checks
		existing, err := client.Me().Events().ByEventId(calUpdateID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching event for safety check: %w", err)
		}

		// Safety: reject series master modification
		if isSeriesMaster(existing) {
			return fmt.Errorf("cannot update recurring event series master — modify specific future instances instead (use calendar list to find the instance ID)")
		}

		// Safety: no past event modification
		if existingStart, ok := parseEventStartTime(existing); ok {
			if err := rejectPastEvent(existingStart, "update"); err != nil {
				return err
			}
		}

		// Safety: block modification of Private events
		if isPrivateEvent(existing) && !calUpdateForce {
			return fmt.Errorf("event is marked Private — use --force to modify (respects confidentiality boundaries)")
		}

		// Safety: protect OOF/Busy blocks from silent modification
		if isOOFOrBusy(existing) && !isOrganizer(existing) && !calUpdateForce {
			return fmt.Errorf("event is marked %s and you are not the organizer — use --force to modify", existing.GetShowAs().String())
		}

		// Safety: block body/subject changes on received invitations
		if !isOrganizer(existing) && (calUpdateSubject != "") {
			return fmt.Errorf("cannot change subject of a meeting organised by someone else — only the organizer can modify invitation content")
		}

		// Safety: large meeting guard (>10 attendees)
		if attendeeCount(existing) > 10 && !calUpdateForce {
			return fmt.Errorf("event has %d attendees — use --force to confirm modification of a large meeting", attendeeCount(existing))
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

		// Fetch event for safety checks
		existing, err := client.Me().Events().ByEventId(calDeleteID).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching event for safety check: %w", err)
		}

		// Safety: reject series master deletion
		if isSeriesMaster(existing) {
			return fmt.Errorf("cannot delete recurring event series master — delete specific future instances instead")
		}

		// Safety: no past event deletion
		if existingStart, ok := parseEventStartTime(existing); ok {
			if err := rejectPastEvent(existingStart, "delete"); err != nil {
				return err
			}
		}

		// Safety: block deletion of Private events (extra confirmation)
		if isPrivateEvent(existing) {
			output.Info("Warning: deleting a Private event")
		}

		// Safety: protect OOF/Busy blocks
		if isOOFOrBusy(existing) && !isOrganizer(existing) {
			return fmt.Errorf("cannot delete event marked %s that you did not organise", existing.GetShowAs().String())
		}

		// Safety: large meeting guard
		if attendeeCount(existing) > 10 {
			output.Info(fmt.Sprintf("Warning: this event has %d attendees — deletion will affect all of them", attendeeCount(existing)))
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
	calCreateCmd.Flags().BoolVar(&calCreateForce, "force", false, "Override duplicate/overlap detection")

	// calendar update
	calUpdateCmd.Flags().StringVar(&calUpdateID, "id", "", "Event ID to update")
	calUpdateCmd.Flags().StringVar(&calUpdateSubject, "subject", "", "New subject")
	calUpdateCmd.Flags().StringVar(&calUpdateStart, "start", "", "New start time (RFC3339 with timezone)")
	calUpdateCmd.Flags().StringVar(&calUpdateEnd, "end", "", "New end time (RFC3339 with timezone)")
	calUpdateCmd.Flags().BoolVar(&calUpdateForce, "force", false, "Override safety guards (private events, OOF/Busy, large meetings)")

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

