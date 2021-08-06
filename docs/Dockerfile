FROM pandoc/latex:2.14.1

ENV TZ=Asia/Jerusalem
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

ENV BOOKDIR /mdbook

VOLUME $BOOKDIR
WORKDIR $BOOKDIR

RUN apk add --no-cache g++ openssl-dev expat cmake curl \
	irrlicht-dev libbz2 \
	sqlite-dev \
	libogg-dev gnutls-dev freetype-dev build-base bzip2-dev expat-dev libxml2-dev \
	graphite2 fontconfig-dev \
	graphite2-dev harfbuzz-dev \
	icu-dev zlib-dev \
	texlive-full

# taken from: https://hub.docker.com/r/k4zuki/pandocker-alpine/dockerfile/

RUN apk add --no-cache fontconfig ttf-dejavu
RUN tlmgr option repository http://mirror.ctan.org/systems/texlive/tlnet
RUN tlmgr update --self && fc-cache -fv && tlmgr install \
    ascmac \
    background \
    bxjscls \
    ctex \
    environ \
    everypage \
    haranoaji \
    haranoaji-extra \
    ifoddpage \
    lastpage \
    mdframed \
    needspace \
    tcolorbox \
    trimspaces \
    xhfill \
    zref \
    zxjafont \
    zxjatype && mktexlsr

RUN curl --proto "=https" --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH /root/.cargo/bin:$PATH

RUN cargo install mdbook
RUN cargo install mdbook-latex

WORKDIR /mdbook
#ENTRYPOINT [ "/usr/bin/mdbook" ]
ENTRYPOINT [ "sh" ]
