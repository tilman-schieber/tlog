# Working on these docs

This book is written in markdown and built with
[mdBook](https://rust-lang.github.io/mdBook/), a single program with no
dependencies. The sources are in the repository under `docs/`.

```text
docs/
├── book.toml        title, where the site is served
├── tlog.css         how the pages look: the palette is at the top
├── tlog.js          colours the pieces of tlog's screen shown in examples
└── src/
    ├── SUMMARY.md   the table of contents — every page is listed here
    ├── index.md
    ├── tutorial/
    ├── concepts/
    ├── reference/
    └── more/
```

A block marked `text` is drawn as a piece of tlog's screen, in a frame, with
bullets, links, tags and checkboxes coloured as the outliner colours them.
Use it for what tlog shows; use `sh`, `markdown` and so on for everything else.

## Previewing

Install mdBook once — `cargo install mdbook`, `mise use -g mdbook`, or your
package manager — then:

```sh
just docs-serve     # http://localhost:3000, reloads as you save
just docs           # build into docs/book/
```

## How the book is organised

- **Tutorial** pages are read in order and tell one story. Each ends with
  links to the concepts it touched.
- **Concepts** pages explain one idea each, and can be read in any order.
- **Reference** pages are for looking things up, and are complete.

When a feature changes, the reference page changes; when the *reason* for
something changes, the concept page does. A new page must be added to
`SUMMARY.md` or it is not built.

## Publishing

### GitHub Pages

The repository has a workflow, `.github/workflows/docs.yml`, that builds the
book and publishes it whenever `docs/` changes on `main`. It needs one switch,
once: in the repository's **Settings → Pages**, set **Source** to **GitHub
Actions**.

### GitLab Pages

Put this in `.gitlab-ci.yml`:

```yaml
pages:
  image: alpine:latest
  variables:
    MDBOOK_VERSION: "0.5.4"
  script:
    - apk add --no-cache curl
    - curl -sSL "https://github.com/rust-lang/mdBook/releases/download/v${MDBOOK_VERSION}/mdbook-v${MDBOOK_VERSION}-x86_64-unknown-linux-musl.tar.gz" | tar -xz
    - ./mdbook build docs --dest-dir "$CI_PROJECT_DIR/public"
  artifacts:
    paths:
      - public
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

### Anywhere else

`just docs` leaves a folder of static files in `docs/book/`. Any web server can
serve it.

If the site is not served from `/tlog/`, change `site-url` in `docs/book.toml`
to match.
