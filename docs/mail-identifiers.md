# Mail evidence and identifiers

`mail get` explicitly requests authentication headers and Internet Message-ID alongside the existing message fields. JSON output exposes only selected authentication/notification headers. These are provider observations; callers must validate the receiving system and domain alignment before trusting a sender. Mail content and identifiers belong in private application state, not public logs.

Use `mail get --id ID --immutable-id --json` to request an immutable identifier in the returned message. The default command retains its existing identifier behavior.

`mail translate-id --id ID --source-type restId --target-type restImmutableEntryId --json` translates exactly one identifier through Microsoft Graph. It uses the selected profile and existing authentication; it does not move, modify or send mail. A translation is accepted only when the response contains exactly one mapping for the supplied source ID. An unavailable mapping is an error, not proof that a message was deleted.

Supported formats are `entryId`, `ewsId`, `immutableEntryId`, `restId`, and `restImmutableEntryId`. See [Microsoft Graph identifier translation](https://learn.microsoft.com/en-us/graph/api/user-translateexchangeids?view=graph-rest-1.0). No authentication scope is added by these commands.
