FROM falcosecurity/event-generator as event-generator
FROM golang:1.23.4 as build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o intruder .

FROM alpine:3
ARG K8SPIDER=k8spider_v3.0.0_linux_amd64.tar.gz
WORKDIR /intruder
RUN apk add curl
# install k8spider
RUN wget https://github.com/Esonhugh/k8spider/releases/download/v3.0.0/$K8SPIDER && \
  tar xvzf $K8SPIDER && \
  mv ./k8spider /usr/bin && \
  rm -rf *
# install falco event-generator
COPY --from=event-generator /bin/event-generator /usr/bin
# install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv ./kubectl /usr/bin

COPY --from=build /app/intruder /usr/bin
ENTRYPOINT [ "intruder", "-c" ]
CMD "echo 'Hello, Mallory!'"