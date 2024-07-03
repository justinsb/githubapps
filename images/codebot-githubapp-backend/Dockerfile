FROM golang:1.22.4 AS build-go

WORKDIR /workspace

ADD go.mod go.mod
ADD go.sum go.sum

ADD api api
ADD cmd cmd
ADD pkg pkg

RUN go build -o /codebot-githubapp-backend ./cmd/githubapp-backend

FROM gcr.io/distroless/static-debian12:latest

COPY --from=build-go /codebot-githubapp-backend /codebot-githubapp-backend

ENTRYPOINT ["/codebot-githubapp-backend"]
