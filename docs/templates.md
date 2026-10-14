# Output formats from templates

A server started with `EMBISTO_TEMPLATES_DIR` makes an output format of every file at the top of that directory named `<name>.<ext>.tmpl`: `slack.json.tmpl` answers `/v1/preview?format=slack`. The name is lowercase letters, digits, `-` and `_`, and cannot be one of embisto's own formats (`json`, `text`, `html`, `iframely`, `oembed`). Every loaded format is listed in the `format` parameter's enum in `/openapi.json`. A request only names a format; template code comes from the directory alone.

The extension, in any case, sets the `Content-Type`. Every answer also carries the `html` format's Content-Security-Policy and `X-Content-Type-Options: nosniff`, so that no output runs a script in a browser.

| Extension | Content-Type |
|---|---|
| `html`, `htm` | `text/html; charset=utf-8` |
| `xhtml` | `application/xhtml+xml; charset=utf-8` |
| `xml` | `application/xml; charset=utf-8` |
| `svg` | `image/svg+xml; charset=utf-8` |
| `json` | `application/json` |
| `md` | `text/markdown; charset=utf-8` |
| `csv` | `text/csv; charset=utf-8` |
| any other, `txt`, `dart`, `rfwtxt` among them | `text/plain; charset=utf-8` |

## Writing a template

Templates use Go's [template language](https://pkg.go.dev/text/template). `.` is the card: the fields of [`card.Preview`](https://github.com/pantafive/embisto/blob/main/card/preview.go) by their Go names, such as `.Title`, `.SiteName`, `.Image.Width` or `.Limited`. A misspelt field fails at startup.

A markup template (`.html`, `.htm`, `.xhtml`, `.xml`, `.svg`) runs as `html/template`, which escapes every value for where it appears. Any other runs as `text/template`, which escapes nothing, so wrap every value from the page in the escaper for the output:

| Helper | Gives |
|---|---|
| `json s` | A JSON string literal, quotes included. |
| `dart s` | A single-quoted Dart string literal; `$` is escaped, so a title never interpolates. |
| `md s` | CommonMark text with every markup character escaped, on one line. |
| `slack s` | Text for Slack's mrkdwn, with `&`, `<` and `>` escaped; still needs `json` around it. |
| `shell s` | One single-quoted POSIX shell word. |
| `truncate n s` | `s` cut at a word to at most `n` characters, with `…` at the cut. |
| `default d s` | `s`, or `d` when `s` is empty. |
| `picture . w webp` | The picture for a box `w` pixels wide: the smallest variant at least that wide, else the largest, else the original; the WebP version when `webp` is true and there is one. It has `.URL`, `.Width`, `.Height` and `.Type`. |
| `ratio w h` | `w/h` as a decimal, for a layout that wants an aspect ratio. |

Helpers reach no file, environment, clock or network.

## Checks at startup

Each template is run on four cards: a full one with thumbnails, an empty one, one behind a login wall, and one whose text holds quotes, a backslash, a newline, `</script>`, a Slack link, `${…}`, U+2028 and an emoji. If any run fails, or the output does not parse as its type (JSON for `.json`, well-formed XML for `.xml`, `.svg` and `.xhtml`, CSV for `.csv`), the server does not start and the error names the file. A template that fails on a real card answers 500 with nothing of its output.

## Examples

[`examples/templates`](https://github.com/pantafive/embisto/blob/main/examples/templates) holds four, each with goldens on real cards under `testdata/`:

- `slack.json.tmpl`: a Block Kit message, the picture as the section's accessory.
- `telegram.html.tmpl`: a message for Telegram's `parse_mode=HTML`, using only `<b>` and `<a>`.
- `markdown.md.tmpl`: a CommonMark snippet.
- `flutter.rfwtxt.tmpl`: a card in the text format of [Remote Flutter Widgets](https://pub.dev/packages/rfw). An app parses it at run time with `parseLibraryFile`, so the card changes without an app release, which a Dart snippet compiled into the app could not. It needs only `core.widgets` registered (`createCoreWidgets()`), and a tap sends the event `open` with the page's `url`. The goldens parse and render with rfw 1.9.1.

```sh
docker run -p 8080:8080 -v "$PWD/examples/templates:/templates:ro" -e EMBISTO_TEMPLATES_DIR=/templates ghcr.io/pantafive/embisto:main
curl 'localhost:8080/v1/preview?format=slack&url=github.com'
```

## In the library

`render.LoadTemplates` takes any `fs.FS`, such as an embedded directory, and returns the formats, to serve beside `render.Defaults()`, with the same checks:

```go
//go:embed templates
var templates embed.FS

sub, err := fs.Sub(templates, "templates")
formats, err := render.LoadTemplates(sub)
renderers := append(render.Defaults(), formats...)
```
