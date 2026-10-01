FROM alpine:3

ARG __golangci_version=v2.11.0

ARG __gid=20
ARG __gname=dev
ARG __uid=501
ARG __uname=dev
ARG __user_home=/home/${__uname}

# ENV GOROOT=/usr/lib/go
# ENV GOPATH=/opt/go
# ENV PATH=${GOPATH}/bin:${GOROOT}/bin:$PATH

RUN apk update --no-cache \
    && apk upgrade --no-cache \
    && apk add --no-cache \
      curl \
      git \
      go \
      make \
      mandoc \
      musl-dev \
      shellcheck \
      zip
#    && mkdir -p ${GOPATH}/src ${GOPATH}/bin \
RUN wget -O- -nv https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
       | sh -s ${__golangci_version}
RUN (getent group ${__gid} | cut -d: -f1 | xargs -I{} adduser -u ${__uid} -G {} -D -h ${__user_home} ${__uname} \
        || (addgroup -g ${__gid} ${__gname} \
            && adduser -u ${__uid} -G ${__gname} -D -h ${__user_home} ${__uname}))

HEALTHCHECK CMD exit 0

USER ${__uname}
