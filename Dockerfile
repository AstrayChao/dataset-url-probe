FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/dataset-probe ./cmd/probe

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/dataset-probe /usr/local/bin/dataset-probe

WORKDIR /data
ENTRYPOINT ["dataset-probe"]
