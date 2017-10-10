package cmd

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"

	pb "github.com/previousnext/m8s/pb"
	"github.com/previousnext/m8s/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type cmdServer struct {
	Port    int32
	TLSCert string
	TLSKey  string

	Token     string
	Namespace string

	FilesystemSize string

	LetsEncryptEmail  string
	LetsEncryptDomain string
	LetsEncryptCache  string

	SSHService string

	DockerCfgRegistry string
	DockerCfgUsername string
	DockerCfgPassword string
	DockerCfgEmail    string
	DockerCfgAuth     string

	PrometheusPort   string
	PrometheusPath   string
	PrometheusApache int32
}

func (cmd *cmdServer) run(c *kingpin.ParseContext) error {
	log.Println("Starting Prometheus Endpoint")

	go metrics(cmd.PrometheusPort, cmd.PrometheusPath)

	log.Println("Starting Server")

	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", cmd.Port))
	if err != nil {
		panic(err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	log.Println("Booting API")

	// Create a new server which adheres to the GRPC interface.
	srv, err := server.New(client, config, cmd.Token, cmd.Namespace, cmd.SSHService, cmd.FilesystemSize, cmd.PrometheusApache, server.DockerRegistry{
		Registry: cmd.DockerCfgRegistry,
		Username: cmd.DockerCfgUsername,
		Password: cmd.DockerCfgPassword,
		Email:    cmd.DockerCfgEmail,
		Auth:     cmd.DockerCfgAuth,
	})
	if err != nil {
		panic(err)
	}

	var creds credentials.TransportCredentials

	// Attempt to load user provided certificates.
	// If no certificates are provided, fallback to Lets Encrypt.
	if cmd.TLSCert != "" && cmd.TLSKey != "" {
		creds, err = credentials.NewServerTLSFromFile(cmd.TLSCert, cmd.TLSKey)
		if err != nil {
			panic(err)
		}
	} else {
		creds, err = getLetsEncrypt(cmd.LetsEncryptDomain, cmd.LetsEncryptEmail, cmd.LetsEncryptCache)
		if err != nil {
			panic(err)
		}
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterM8SServer(grpcServer, srv)
	return grpcServer.Serve(listen)
}

// Server declares the "server" sub command.
func Server(app *kingpin.Application) {
	c := new(cmdServer)

	cmd := app.Command("server", "Run the M8s server").Action(c.run)
	cmd.Flag("port", "Port to run this service on").Default("443").OverrideDefaultFromEnvar("M8S_PORT").Int32Var(&c.Port)
	cmd.Flag("cert", "Certificate for TLS connection").Default("").OverrideDefaultFromEnvar("M8S_TLS_CERT").StringVar(&c.TLSCert)
	cmd.Flag("key", "Private key for TLS connection").Default("").OverrideDefaultFromEnvar("M8S_TLS_KEY").StringVar(&c.TLSKey)

	cmd.Flag("token", "Token to authenticate against the API.").Default("").OverrideDefaultFromEnvar("M8S_AUTH_TOKEN").StringVar(&c.Token)
	cmd.Flag("namespace", "Namespace to build environments.").Default("default").OverrideDefaultFromEnvar("M8S_NAMESPACE").StringVar(&c.Namespace)

	cmd.Flag("fs-size", "Size of the filesystem for persistent storage").Default("100Gi").OverrideDefaultFromEnvar("M8S_FS_SIZE").StringVar(&c.FilesystemSize)

	// Lets Encrypt.
	cmd.Flag("lets-encrypt-email", "Email address to register with Lets Encrypt certificate").Default("admin@previousnext.com.au").OverrideDefaultFromEnvar("M8S_LETS_ENCRYPT_EMAIL").StringVar(&c.LetsEncryptEmail)
	cmd.Flag("lets-encrypt-domain", "Domain to use for Lets Encrypt certificate").Default("").OverrideDefaultFromEnvar("M8S_LETS_ENCRYPT_DOMAIN").StringVar(&c.LetsEncryptDomain)
	cmd.Flag("lets-encrypt-cache", "Cache directory to use for Lets Encrypt").Default("/tmp").OverrideDefaultFromEnvar("M8S_LETS_ENCRYPT_CACHE").StringVar(&c.LetsEncryptCache)

	// SSH Server.
	cmd.Flag("ssh-service", "SSH server image to deploy").Default("ssh-server").OverrideDefaultFromEnvar("M8S_SSH_SERVICE").StringVar(&c.SSHService)

	// DockerCfg.
	cmd.Flag("dockercfg-registry", "Registry for Docker Hub credentials").Default("").OverrideDefaultFromEnvar("M8S_DOCKERCFG_REGISTRY").StringVar(&c.DockerCfgRegistry)
	cmd.Flag("dockercfg-username", "Username for Docker Hub credentials").Default("").OverrideDefaultFromEnvar("M8S_DOCKERCFG_USERNAME").StringVar(&c.DockerCfgUsername)
	cmd.Flag("dockercfg-password", "Password for Docker Hub credentials").Default("").OverrideDefaultFromEnvar("M8S_DOCKERCFG_PASSWORD").StringVar(&c.DockerCfgPassword)
	cmd.Flag("dockercfg-email", "Email for Docker Hub credentials").Default("").OverrideDefaultFromEnvar("M8S_DOCKERCFG_EMAIL").StringVar(&c.DockerCfgEmail)
	cmd.Flag("dockercfg-auth", "Auth token for Docker Hub credentials").Default("").OverrideDefaultFromEnvar("M8S_DOCKERCFG_AUTH").StringVar(&c.DockerCfgAuth)

	// Promtheus.
	cmd.Flag("prometheus-port", "Prometheus metrics port").Default(":9000").OverrideDefaultFromEnvar("M8S_METRICS_PORT").StringVar(&c.PrometheusPort)
	cmd.Flag("prometheus-path", "Prometheus metrics path").Default("/metrics").OverrideDefaultFromEnvar("M8S_METRICS_PATH").StringVar(&c.PrometheusPath)
	cmd.Flag("prometheus-apache-exporter", "Prometheus metrics port for Apache on built environments").Default("9117").OverrideDefaultFromEnvar("M8S_METRICS_APACHE_PORT").Int32Var(&c.PrometheusApache)
}

// Helper function for serving Prometheus metrics.
func metrics(port, path string) {
	http.Handle(path, promhttp.Handler())
	log.Fatal(http.ListenAndServe(port, nil))
}

// Helper function for adding Lets Encrypt certificates.
func getLetsEncrypt(domain, email, cache string) (credentials.TransportCredentials, error) {
	manager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(cache),
		HostPolicy: autocert.HostWhitelist(domain),
		Email:      email,
	}

	return credentials.NewTLS(&tls.Config{GetCertificate: manager.GetCertificate}), nil
}