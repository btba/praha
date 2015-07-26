# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang

# Copy the local package files to the container's workspace.
ADD . /go/src/github.com/btba/praha/gorez

# Build the outyet command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go get github.com/go-sql-driver/mysql
RUN go install github.com/btba/praha/gorez

COPY templates /gorez/templates

# Run the outyet command by default when the container starts.
ENTRYPOINT /go/bin/gorez \
  --bookings_dsn=${BOOKINGS_DSN} \
  --templates_dir=/gorez/templates

# Document that the service listens on port 8080.
EXPOSE 8080
