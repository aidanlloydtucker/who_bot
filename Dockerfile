FROM golang:1.7

ARG version

COPY . /go/src/github.com/billybobjoeaglt/who_bot/
WORKDIR /go/src/github.com/billybobjoeaglt/who_bot/
RUN make build VERSION=$version

ENTRYPOINT ["/go/src/github.com/billybobjoeaglt/who_bot/who_bot"]
CMD ["--help"]
