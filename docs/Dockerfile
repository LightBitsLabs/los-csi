FROM peaceiris/mdbook

RUN apk add vim cargo
ENV PATH="${PATH}:/root/.cargo/bin"
RUN cargo install mdbook-toc
#RUN wget https://github.com/shonfeder/emojitsu/releases/download/0.1.1/gh-actions-emojitsu && chmod +x gh-actions-emojitsu
#RUN cargo install mdbook-plantuml
