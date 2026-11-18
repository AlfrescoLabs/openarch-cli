# Permission demo: same query, different users

Hyland OpenArch answers a query with only the documents that the caller may read. This guide shows it
with `openarch demo`: you run the same query as four users and compare the results. You need Docker and
the CLI for OpenArch. You do not need Alfresco or an AI model.

Tested with CLI for OpenArch 0.9.0 and Hyland OpenArch 1.0.103 on macOS (arm64), on 2026-11-18.

The video [Docker Extension for OpenArch 0.1 and CLI for OpenArch 0.6](https://youtu.be/GD_HLXCmYTA)
shows an older version of this demo, with less sample data:

| Demo | Documents | admin | alice | bob | carol |
| --- | --- | --- | --- | --- | --- |
| CLI 0.6.0 (video) | 6 | 6 | 4 | 3 | not in the demo |
| CLI 0.8.0 and 0.9.0 (this guide) | 10 | 10 | 6 | 6 | 2 |

## 1. Prerequisites

- Docker with Docker Compose, with about 4 GB of memory free for this stack.
- CLI for OpenArch 0.9.0, from the [v0.9.0 release](https://github.com/AlfrescoLabs/openarch-cli/releases/tag/v0.9.0).
- Port 18088 free. This guide uses it so that it does not clash with a workspace on port 8080.
- A terminal with bash or zsh, on macOS or Linux.

```
openarch version
```

The counts in this guide are for 0.9.0. Another release can have other sample data: see
[Demo](../README.md#demo) in the README of that release. The first `up` pulls the container images, so
it takes a few minutes.

## 2. Create a separate workspace

Run all blocks in the same terminal. The temporary folder keeps the saved connections and the passwords
of this demo away from your other workspaces.

```
DEMO_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/openarch-permissions.XXXXXX")
DEMO_WORKSPACE="$DEMO_ROOT/oa-permissions-$(basename "$DEMO_ROOT")"
export OPENARCH_CONFIG_DIR="$DEMO_ROOT/config"
export OPENARCH_CREDENTIAL_STORE=file

openarch init --dir "$DEMO_WORKSPACE" \
  --context-name permissions --version 1.0.103 \
  --port 18088 --memory 4g --non-interactive

openarch demo users --dir "$DEMO_WORKSPACE"
openarch up --dir "$DEMO_WORKSPACE"
openarch demo load --dir "$DEMO_WORKSPACE"
```

- `demo users` adds alice, bob and carol, and saves a context for each one. Run it before `up`:
  Hyland OpenArch reads its users only at startup.
- `demo load` adds folders and documents under `/Demo`. The documents are synthetic and have metadata
  only (title and description).
- The workspace and `$DEMO_ROOT/config` hold the generated passwords in clear text. Keep them private.

## 3. Run the same query as each user

```
DEMO_QUERY="SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'"

openarch query "$DEMO_QUERY" --context permissions --sync --limit 50
openarch query "$DEMO_QUERY" --context permissions-alice --sync --limit 50
openarch query "$DEMO_QUERY" --context permissions-bob --sync --limit 50
openarch query "$DEMO_QUERY" --context permissions-carol --sync --limit 50
```

`--sync` waits for the search index. `--limit 50` is larger than the demo, so every result fits on one
page. The `permissions` context is the admin.

alice gets this (your IDs differ):

```
115c5554-...  SysFile  /Demo/Public/holiday-calendar-2026  (Holiday calendar 2026)
db3c1f28-...  SysFile  /Demo/Public/office-wifi-guide  (Office Wi-Fi guide)
4ed8cf03-...  SysFile  /Demo/HR/salary-bands-2026  (Salary bands 2026)
a93e58c1-...  SysFile  /Demo/HR/hiring-plan-q1-2027  (Hiring plan Q1 2027)
8848d7c3-...  SysFile  /Demo/HR/Reviews/review-calendar-2027  (Review calendar 2027)
d9a867c9-...  SysFile  /Demo/Finance/travel-policy-2027  (Travel policy 2027)

1 to 6 of 6
```

| User | Count | Documents |
| --- | --- | --- |
| admin | 10 | all ten |
| alice | 6 | Holiday calendar 2026, Office Wi-Fi guide, Salary bands 2026, Hiring plan Q1 2027, Review calendar 2027, Travel policy 2027 |
| bob | 6 | Holiday calendar 2026, Office Wi-Fi guide, Office move plan, Budget forecast 2027, Travel policy 2027, Contractor rates 2027 |
| carol | 2 | Holiday calendar 2026, Office Wi-Fi guide |

alice and bob both get 6, but not the same 6. Compare the titles, not only the counts.

## 4. Why each user sees what they see

- Every demo user is in the group `members`, which can read the two documents in `/Demo/Public`.
- alice is in `demo-hr`. She reads the HR documents, and the review calendar in `/Demo/HR/Reviews`
  through inheritance.
- bob is in `demo-finance`. He reads the Finance documents.
- Office move plan is in `/Demo/Public/Facilities`. That folder blocks what it inherits and gives access
  to bob only. A folder under Public is not public by default.
- Travel policy 2027 is in Finance, but it also gives access to alice.
- alice had access to Contractor rates 2027 until 2026-01-01. The access has ended, so she does not get it.
- Merger proposal is for administrators only. No demo user gets it.

## 5. Try a narrower query

```
DEMO_2027_QUERY="SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/' AND sys_title LIKE '%2027%'"

openarch query "$DEMO_2027_QUERY" --context permissions-alice --sync --limit 50
openarch query "$DEMO_2027_QUERY" --context permissions-bob --sync --limit 50
openarch query "$DEMO_2027_QUERY" --context permissions-carol --sync --limit 50
```

- alice: Hiring plan Q1 2027, Review calendar 2027, Travel policy 2027.
- bob: Budget forecast 2027, Travel policy 2027, Contractor rates 2027.
- carol: `No documents.`

The filter acts on top of the permissions: a user never gets a document they may not read.

## 6. Check every user at once

```
openarch demo check --dir "$DEMO_WORKSPACE"
```

`demo check` logs in as the admin, alice, bob and carol, and compares what Hyland OpenArch returns with
the access entries of the demo: document lists from the search index and from the database, title
queries, and a read of every document by id. It prints one line per user and a result line:

```
admin  may read 10 of 10 documents: 22 requests, 0 leaks, 0 problems
alice  may read  6 of 10 documents: 22 requests, 0 leaks, 0 problems
bob    may read  6 of 10 documents: 22 requests, 0 leaks, 0 problems
carol  may read  2 of 10 documents: 22 requests, 0 leaks, 0 problems

PASS: 4 users, 10 documents, 88 requests (lists, title queries, reads), 0 leaks, 0 problems. CLI for OpenArch 0.9.0, Hyland OpenArch 1.0.103.
```

A leak is the name, the code or the content of a document the user may not read. A problem is a
document the user may read but does not get. The exit code is 0 when there is neither.

## 7. Stop or remove the demo

Stop the containers and keep the data:

```
openarch down --dir "$DEMO_WORKSPACE"
```

Or remove the containers and the data of this demo:

```
openarch clean --dir "$DEMO_WORKSPACE" --yes
```

Both act only on this workspace. When you are done, delete `$DEMO_ROOT` too: it holds the passwords.

```
rm -rf "$DEMO_ROOT"
```

## Troubleshooting

| Problem | What to check |
| --- | --- |
| Cannot connect to the Docker daemon | Start Docker. Check that `docker ps` works in the same terminal. |
| Port 18088 is in use | Pick another free port in `init`, before `up`. |
| A demo user gets an authentication error | `demo users` ran after `up`. Run `openarch down` and `openarch up` for this workspace, so that Hyland OpenArch reads the users again. |
| `No documents.` for every user | Check that `demo load` finished, that you used `--sync`, and the `permissions-*` context name. |
| Counts differ from the video | The video uses CLI 0.6.0. See the table at the top. |

## Next steps

- Share your result: leave a comment under [the video](https://youtu.be/GD_HLXCmYTA) with the last
  line of `demo check`.
- Something failed? [Open an issue](https://github.com/AlfrescoLabs/openarch-cli/issues) with the
  command and the error. Remove the passwords first.
- Semantic search on the same demo, with a local model: see [Semantic search](../README.md#semantic-search).
- More videos: the [OpenArch playlist](https://www.youtube.com/playlist?list=PLDj9E8gX-XFo).
