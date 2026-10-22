FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /zarba .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /zarba /zarba
ENV ADDR=:8080
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/zarba"]
