module github.com/erda-project/remotedialer/server

go 1.16

require (
	github.com/gorilla/mux v1.7.3
	github.com/rancher/remotedialer v0.0.0
	github.com/sirupsen/logrus v1.4.2
)

replace github.com/rancher/remotedialer => ../
