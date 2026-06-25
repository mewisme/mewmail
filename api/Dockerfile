FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM scratch

COPY --from=build /server /server

USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/server"]
