FROM golang:1.21.6 as build

WORKDIR /node

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 go build -ldflags '-s' -o /node/main

FROM golang:1.21.6

RUN apt-get update && apt-get install cron -y

WORKDIR /node

COPY --from=build /node/main /node/main
COPY ids.json /node/ids.json

COPY cron_runner.sh .

RUN chmod +x cron_runner.sh

EXPOSE 5000

ENTRYPOINT ["./cron_runner.sh"]