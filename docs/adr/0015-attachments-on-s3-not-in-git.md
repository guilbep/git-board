# ADR 0015: Attachments go to an S3 bucket behind a CDN, not into git

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: before 2026-10-20 (the shallow clone's history starts there and
  `internal/uploads` is older; no doc dates it). ADR 0002 lists it as
  "already outside git".
- Sources: `internal/uploads/uploads.go`; `README.md` › Attachments;
  `docs/guide/configuration.md` › Attachments.
- No doc records the alternatives: the table below is reconstructed.

## Context

Comments carry screenshots, PDFs and logs. The board is kept as operations
in git (ADR 0004) and mirrored to GitHub and GitLab (ADR 0008). A file in
a comment has to show in git-board, on GitHub and on GitLab alike, and
must not bloat every clone's history. GitHub's own attachments to a private
repository need a GitHub login to view, so a link copied to GitLab would
break.

## Decision

**A file pasted, dropped or picked in a comment is uploaded to an S3
bucket a CDN serves; the comment gets plain Markdown linking to it.**

- One package, `internal/uploads`, signs requests with AWS Signature V4 in
  a few lines of the standard library, not an SDK
  (`internal/uploads/uploads.go:1-6`, `:176` `sign`; checked against AWS's
  own example in `uploads_test.go`).
- Defaults are EPFL ENAC's: bucket on `https://s3.epfl.ch`, served by
  `https://enacit4r-cdn.epfl.ch`, under `git-board/<repository>`
  (`docs/guide/configuration.md` › Attachments). `[uploads]` in
  `.git-board.toml` changes the addresses and the prefix.
- Keys come only from the server's environment: `ACCESS_KEY_ID`,
  `ACCESS_KEY_SECRET`, `BUCKET_NAME` (`internal/uploads/uploads.go:47-52`).
  Without them attaching is off (`:228` `ErrDisabled`).
- A file's key is `prefix/YYYY-MM/<random>/<name>`: the bucket is public
  but not browsable, so the random part is the only protection
  (`:147-150` `key`, 8 random bytes).
- 25 MB at most, GitHub's own limit (`:34` `MaxBytes`); images (not SVG),
  PDF, video, text, CSV, zip and office files (`:113` `Check`).
- Uploads go through the server's guard (ADR 0013): a multipart body needs
  the page's `X-Git-Board-Upload` header.

## Alternatives considered

Reconstructed. Scored on: the file shows on every forge without a login
(30), clones stay small (25), no new service to run (25), privacy (20).

| Option | Score | Why not |
| --- | --- | --- |
| **S3 behind a CDN, public by unguessable link (this decision)** | 70 | |
| Files as blobs in the board's git refs | 45 | Every clone fetches every file forever; forges can't show them |
| Git LFS | 40 | Needs LFS on every remote and client; forges don't render LFS objects inside issue comments |
| The forge's own attachment upload | 45 | Tied to one forge; GitHub's private-repository attachments need a login; nothing to upload to with no forge |
| A private bucket with signed, expiring URLs | 50 | Links in forge comments would expire |

## Consequences

Good:

- A file shows the same in git-board, on GitHub and on GitLab.
- Clones carry links, not bytes.
- No SDK dependency.

Bad, and accepted:

- **Files are public to anyone with the link, even for a private
  repository**, and a deleted file stays cached on the CDN for up to a
  year (`README.md` › Attachments).
- The board depends on a service outside git: a board moved elsewhere
  keeps links to EPFL's CDN.
- Each server that accepts uploads needs the keys in its environment.

## Left open

- The S3 service the keys go to is the server's, not the repository's:
  `UPLOADS_ENDPOINT`, else EPFL's (since 2026-11-04). A `[uploads] endpoint`
  in `.git-board.toml` that differs turns uploads off, so a repository
  can't send the operator's keys elsewhere.
- The defaults are EPFL's. `docs/prd/ui-review.md` D-27 kept them, "the
  uploads defaults are behaviour", and named them in the UI; whether a
  non-EPFL install should start with uploads off and no default endpoint
  is open.
- Files are not copied by `git board datastore copy`, the Markdown export,
  or the switch: they stay where they are, linked.
- This sits uneasily with the vision of full ownership of the board: the
  issue text is owned, its attachments are rented.
