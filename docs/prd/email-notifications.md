# Email notifications

An investigation to turn into a plan. Start a session here with:

```sh
cd ~/works/git/github/git-board-notify && claude
# then: "Read docs/prd/email-notifications.md, and let's investigate it."
```

## Why

git remotes carry everything (code, branches, issues): issues travel with
`git bug push/pull <remote>`, forges are optional mirrors. What people
actually like about forges' email is being told when something happens on
an issue they follow, and answering by replying to the email. Patches by
email were set aside for the team (Outlook mangles them, no review page).

## Questions to investigate

1. **Who follows what**: the author, the assignees, the people in a role
   field (pm, reviewer, lead dev), anyone who commented, anyone who
   subscribed. Where is a subscription stored: a label, git-bug metadata,
   the hosted server's own state?
2. **What triggers an email**: new comment, status change, assigned to
   you, mentioned, a sync conflict, a sprint ending. Digest or one per
   event?
3. **Who sends**: only a hosted git-board can (a laptop can't be expected
   to); EPFL's SMTP relay, or Microsoft 365 / Entra.
4. **Replies by email**: an inbox (IMAP, or a Microsoft Graph mailbox) read
   by the server, each reply turned into a comment by the right identity.
   How is the sender authenticated, and the quoted text stripped?
5. **As a bridge?** The sync engine's adapter interface (list, fetch,
   create, comment, set fields) could treat a mailbox as a forge: issue ↔
   thread, comment ↔ reply. Worth it, or plain notifications only?
6. **Teams or Slack** instead of, or beside, email.
7. **Privacy**: email addresses of identities, opt-out, what an email may
   contain for a private repository.

## What exists to build on

- git-board-server streams changes live (`/api/events`, SSE) and knows
  "since your last visit"; the hosted PRD ("git-board hosted: PRD and
  plan") plans OIDC login (Entra/Keycloak) and roles.
- git-bug identities carry a name, an email and forge logins.

## Constraints

- Never send real emails to the team while trying it: a test inbox only.
- co2-calculator stays local; GitHub Project 40 is read-only.
