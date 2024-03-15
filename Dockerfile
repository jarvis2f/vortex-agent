FROM golang:1.21.5-alpine3.18 as builder

WORKDIR /app

ENV GOPATH /go
ENV GOROOT /usr/local/go
ENV PATH $PATH:/go/bin:$GOPATH/bin

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN go build -o ./vortex

FROM alpine:3.18 as runner
RUN apk add --no-cache bash python3
WORKDIR /app

COPY --from=builder /app/vortex ./files
COPY ./scripts/*.sh ./files
COPY ./scripts/server.py .

CMD ["./server.py"]
