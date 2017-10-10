FROM golang:1.8
ADD workspace /go
RUN go get github.com/mitchellh/gox
RUN make api

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=0 /go/bin/m8s-api_linux_amd64 /usr/local/bin/m8s
CMD ["m8s"]