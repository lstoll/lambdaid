package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/apex/gateway"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/lstoll/awskms"
	"github.com/pardot/oidc/discovery"
	"gopkg.in/square/go-jose.v2"
)

var (
	// DefaultHTTPGetAddress Default Address
	DefaultHTTPGetAddress = "https://checkip.amazonaws.com"

	// ErrNoIP No IP found in response
	ErrNoIP = errors.New("No IP in HTTP response")

	// ErrNon200Response non 200 status code in response
	ErrNon200Response = errors.New("Non 200 Response found")
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	lreq, ok := gateway.RequestContext(r.Context())
	if ok {
		log.Printf("Processing request data for request %s.\n", lreq.RequestID)
	}
	log.Printf("Path: %s\n", r.URL.Path)
	log.Printf("Query: %v\n", r.URL.Query())

	// fmt.Printf("Body size = %d.\n", len(request.Body))

	log.Println("Headers:")
	for key, value := range r.Header {
		fmt.Printf("    %s: %s\n", key, value)
	}

	resp, err := http.Get(DefaultHTTPGetAddress)
	if err != nil {
		log.Printf("error in get: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != 200 {
		log.Printf("error in get: %v", ErrNon200Response)
		http.Error(w, ErrNon200Response.Error(), http.StatusInternalServerError)
		return
	}

	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error in get: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(ip) == 0 {
		log.Printf("error in get: %v", ErrNoIP)
		http.Error(w, ErrNoIP.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Hello, %v", string(ip))
}

func main() {
	ctx := context.Background()

	var (
		baseURL                = os.Getenv("BASE_URL")
		oidcSignerKMSARN       = os.Getenv("KMS_OIDC_KEY_ARN")
		oidcSignerGenerateWeak = os.Getenv("OIDC_SIGNER_GENERATE_WEAK")
	)

	sess, err := session.NewSession(&aws.Config{})
	if err != nil {
		log.Fatalf("creating aws sdk session: %v", err)
	}
	kmscli := kms.New(sess)

	var (
		jwtSigner crypto.Signer
		jwtKeyID  string
	)

	log.Printf("env: %v", os.Environ())
	log.Printf("genweah: %s", oidcSignerGenerateWeak)

	if oidcSignerGenerateWeak == "true" || oidcSignerGenerateWeak == "1" {
		log.Print("in genweak block")
		// generate a crappy local key, for development purposes. Never erver
		// use this live.
		k, err := rsa.GenerateKey(rand.Reader, 512)
		if err != nil {
			log.Fatalf("generating RSA key: %v", err)
		}
		jwtSigner = k
		jwtKeyID = time.Now().String()
	} else {
		if oidcSignerKMSARN == "" {
			log.Fatal("KMS_OIDC_KEY_ARN must be set")
		}
		// Use the KMS key
		s, err := awskms.NewSigner(ctx, kmscli, oidcSignerKMSARN)
		if err != nil {
			log.Fatalf("creating KMS signer: %v", err)
		}
		jwtSigner = s
		jwtKeyID = oidcSignerKMSARN
	}

	// hash the key ID, to make it not easily reversable
	kh := sha256.New()
	if _, err := kh.Write([]byte(jwtKeyID)); err != nil {
		log.Fatal(err)
	}
	jwtKeyID = hex.EncodeToString(kh.Sum(nil))[0:16]

	sks := &signerToKeySource{Signer: jwtSigner, keyID: jwtKeyID}
	oidcmd := discovery.ProviderMetadata{
		Issuer:                baseURL,
		JWKSURI:               baseURL + "/keys",
		AuthorizationEndpoint: baseURL + "/auth",
		TokenEndpoint:         baseURL + "/token",
	}
	keysh := discovery.NewKeysHandler(sks, 1*time.Hour)
	discoh, err := discovery.NewConfigurationHandler(&oidcmd, discovery.WithCoreDefaults())
	if err != nil {
		log.Fatalf("configuring metadata handler: %v", err)
	}

	m := http.NewServeMux()

	m.HandleFunc("/hello", helloHandler)
	m.Handle("/keys", keysh)
	m.Handle("/.well-known/openid-configuration", discoh)
	gateway.ListenAndServe("", m)
}

type signerToKeySource struct {
	crypto.Signer
	keyID string
}

func (s *signerToKeySource) PublicKeys(ctx context.Context) (*jose.JSONWebKeySet, error) {
	return &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				Key:       s.Public(),
				KeyID:     s.keyID,
				Algorithm: "RS256",
				Use:       "sig",
			},
		},
	}, nil
}
