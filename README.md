# amethyst-docs-builder

This is a simple go server that listens on a specified endpoint, runs a script and invalidates the cloudfront distribution when a push to master happens. The following environment variables are required:

```bash
SECRET # github webhook secret, should be generated randomly
PORT # port where the server will, uh, serve
SCRIPT # script that should be run, will default to the included one

DOCS_URL # the url of the docs host, e.g. docs.amethyst.rs (used for routing)
BOOK_URL # the url of the book host, e.g. book.amethyst.rs (used for routing)
DOCS_BASE_URL # the base url for the docs (used for redirecting)
BOOK_BASE_URL # the base url for the book (used for redirecting)

TRIGGER_URL # the full url of the hook that will be called, e.g. hook.amethyst.rs

# aws credentials
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY

DOCS_CDN_DIST_ID # cloudfront distribution id for the docs
BOOK_CDN_DIST_ID # cloudfront distribution id for the book
```
