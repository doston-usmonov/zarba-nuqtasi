FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY migrations ./migrations
COPY web ./web
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /zarba . \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /zarba /zarba
# oldingi versiyaning fayl bazasi (birinchi ishga tushishda PostgreSQL ga import qilinadi)
COPY --from=build --chown=65532:65532 /out/data /data
ENV ADDR=:8080 DATA_DIR=/data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/zarba"]
