we save some information in a metadata file named: `metadata.md`

you can write the default template of the latex using the following command to file:

```bash
pandoc -D latex > src/templates/default.latex
```

Then we can modify it and use the new modified custom template like so:

```bash
pandoc --template=src/templates/default.latex
```

In order to see how the Latex looks using the `metadata.md` we can run the following command:

```bash
pandoc -s metadata.md -o metadata.tex
```

to generate the book at first:

```bash
RUST_BACKTRACE=full mdbook build
```

full command to generate the book:

```bash
pandoc --pdf-engine=xelatex --toc --from=markdown --to latex ./book/latex/LightOS\ CSI\ Book.md -o ./book/latex/lb-csi-plugin-deployment-guide-v1.pdf --wrap=none  -V block-headings --preserve-t
abs --metadata-file=metadata1.md --template=src/templates/default.latex --listings --lua-filter ./lua-support-yaml-lang.lua
```


page .40 table is not good line is one over the other!
footer color.
cover should appear before TOC.
JWT line wrap.
