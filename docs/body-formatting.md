# Title and body formatting

planesync mirrors an item's title, status, and body. This describes the title
and body; status is covered in [status-mapping.md](status-mapping.md).

## Title

The destination summary is `"[<prefix>] <source title>"`, where `title_prefix`
is configured per project. An empty prefix yields the title unchanged.

## Body

The destination description is Atlassian Document Format (ADF) and always ends
with `Mirrored from Plane: <identifier>`, where the identifier is the source
item's human-readable project identifier and sequence number (for example,
`SRC-16`). The footer is plain text, not a hyperlink. Two modes, selected by
`defaults.body_format`:

- `rich` — the source HTML is converted to ADF over a deterministic subset:
  paragraphs, line breaks, headings, ordered and bullet lists, links, and
  strong/emphasis/code/pre marks. Unknown tags degrade to their text content, so
  conversion never fails. Code spans emit an exclusive `code` mark, as required
  by ADF, rather than inheriting strong or emphasis marks.
- `text` — the source HTML is stripped to plain text and wrapped in ADF
  paragraphs.

The description is overwritten on every run; the destination is a read-only
reflection, so manual edits do not survive.
