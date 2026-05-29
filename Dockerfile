FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN for bin in dispatcher gatherer researcher planner designer coder committer; do \
      CGO_ENABLED=0 go build -o /out/$bin ./cmd/$bin/; \
    done

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/* /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/dispatcher"]
