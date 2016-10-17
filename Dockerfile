FROM       ubuntu:16.04


RUN apt-get update
RUN apt-get -y upgrade
RUN apt-get -y install golang

RUN mkdir /usr/local/challenge
ENV GOPATH=/usr/local/challenge

EXPOSE 7777
ADD . /usr/local/challenge/
CMD go run $GOPATH/src/db/server/server.go
