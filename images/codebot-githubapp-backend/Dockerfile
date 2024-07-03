FROM golang:1.22.4 AS build-go

WORKDIR /workspace

ADD go.mod go.mod
ADD go.sum go.sum

RUN CGO_ENABLED=0 go build k8s.io/api/...

ADD api api
ADD cmd cmd
ADD pkg pkg

RUN CGO_ENABLED=0 go build -o /codebot-githubapp-backend ./cmd/githubapp-backend

FROM gcr.io/distroless/static-debian12:latest

COPY --from=build-go /codebot-githubapp-backend /codebot-githubapp-backend

ENTRYPOINT ["/codebot-githubapp-backend"]
