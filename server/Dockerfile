FROM golang AS builder
ENV GOBIN /go/bin
#RUN mkdir /app && \
#  mkdir -p /go/src/github.com/mtfelian && \
#  cd /go/src/github.com/mtfelian && \
#  git clone https://github.com/mtfelian/try-kafka && \
#  cd /go/src/github.com/mtfelian/try-kafka && \
#  go build -o /app/server ./server
RUN mkdir -p /app/src && mkdir -p /go/src/github.com/mtfelian/try-kafka/server
COPY . /go/src/github.com/mtfelian/try-kafka/server
RUN cd /go/src/github.com/mtfelian/try-kafka/server && go build -o /app/server ./

FROM debian
RUN apt update && apt upgrade -y && mkdir /app
COPY --from=builder /app/server /app/server
WORKDIR /app
ENTRYPOINT ["/app/server"]
