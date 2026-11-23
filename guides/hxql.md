# HxQL guide

HxQL is the query language of Hyland OpenArch. The `query` tool runs it as the
user of this connection. The result has only the documents that this user may
read. No documents is a valid answer: the documents do not exist, or the user
may not read them.

## Shape

```
SELECT * FROM <type> [WHERE <condition>] [ORDER BY <field> [DESC]]
```

- Always write `SELECT *`. A query that names columns, such as
  `SELECT sys_title FROM SysFile`, returns no documents.
- Types: `SysContent` (folders and files), `SysFolder`, `SysFile`. An unknown
  type is an error.

## Fields

`sys_title`, `sys_name`, `sys_description`, `sys_path`, `sys_parentPath`,
`sys_id`, `sys_parentId`, `sys_primaryType`, `sys_created`, `sys_modified`.

## Conditions

| What | Example |
| --- | --- |
| Below a folder, at any depth | `sys_path STARTSWITH '/Demo/'` |
| Directly in a folder | `sys_parentPath = '/Demo/HR'` |
| Exact value | `sys_title = 'Salary bands 2026'` |
| One of some values | `sys_title IN ('Salary bands 2026', 'Merger proposal')` |
| Part of a value | `sys_title LIKE '%Salary%'` |
| Not | `sys_title NOT LIKE '%2027%'`, `sys_title <> 'Merger proposal'` |
| Date | `sys_created > DATE '2026-01-01'`, `sys_modified > TIMESTAMP '2026-01-01T00:00:00Z'` |
| Words in title or description | `sys_fulltext = 'salary'` |

- Join conditions with `AND` and `OR`.
- `LIKE` is case-sensitive: `'%salary%'` does not find "Salary bands 2026".
  Try the word as written and capitalized, or use `sys_fulltext`.
- Write `sys_title NOT LIKE ...`, not `NOT sys_title LIKE ...`: the second form
  fails with `sync`.
- Write dates with `DATE '...'` or `TIMESTAMP '...'`. A plain string works only
  without `sync`.
- `sys_fulltext` finds words in the title and the description, in any case and
  in other forms ("holidays" finds "Holiday"). It does not read the text inside
  files.

## sync

`sync: true` waits until the search index has the latest changes. Use it right
after a document was created or changed. Some conditions work only with one
setting:

| Condition | without sync (default) | with sync |
| --- | --- | --- |
| `sys_fulltext = '...'` | works | error "Fulltext search not available" |
| `ILIKE` (LIKE in any case) | no documents | works |
| Date as a plain string | works | no documents |
| `NOT` before a condition | works | server error |

## Results

- `limit` is the number of documents (default 10). `offset` skips documents, for
  the next page. `total: true` counts every match.
- `ORDER BY sys_title` or the `sort` parameter sets the order.
- `semantic` finds documents near in meaning to a text, when the documents have
  vectors. The query then selects the documents to search, for example
  `SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'`.
