# CSI Documentation

## Usage

We need to take a couple of steps to generate the documentations of the LB-CSI plugin.

## Docker Image for Documentation Generation

We use [mdbook](https://github.com/rust-lang/mdBook) and [pandoc](https://pandoc.org/) for generating the documentation.

There are a lot of dependencies to be able to generate these books so we work with a Docker image to bring all deps.

```bash
make -f docs/Makefile.docs build-image
```

Generate website under `docs/book/html`

```bash
make -f docs/Makefile.docs mdbook-build
```

Generate Markdown `docs/src/book/latex/LightOS\ CSI\ Book.md` and PDF `docs/src/book/latex/lb-csi-plugin-deployment-guide-v1.pdf`:

```bash
make -f docs/Makefile.docs pandoc-pdf
```
