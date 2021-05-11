module github.com/erda-project/remotedialer/proxyclient

go 1.16

require (
	github.com/go-sql-driver/mysql v1.6.0
	github.com/gorilla/websocket v1.4.0
	github.com/rancher/remotedialer v0.0.0
	github.com/sirupsen/logrus v1.4.2
)

replace github.com/rancher/remotedialer => ../
