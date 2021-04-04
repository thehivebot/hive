module github.com/thehivebot/hive

go 1.16

// etcd fix
replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/bwmarrin/discordgo v0.23.3-0.20210314162722-182d9b48f34b
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/meyskens/discord-ha v0.0.0-20210315192353-c63c44a23a77
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	go.mongodb.org/mongo-driver v1.5.1
)
