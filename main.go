// Service to randomise the value of a salt variable on algolia index
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/alsco77/etherscan-api"
	sendgrid "github.com/canyaio/sendgrid-go"
	"github.com/canyaio/sendgrid-go/helpers/mail"
	"github.com/thoas/go-funk"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

type address struct {
	Addr string `json:"addr"`
	Name string `json:"name"`
}

var (
	startBlock       *int
	minutes          int
	etherscanAPIKey  string
	sendgridAPIKey   string
	sendgridTemplate string
	addresses        []address
)

func init() {
	etherscanAPIKey = getEnv("ETHERSCAN_API_KEY", "")
	sendgridAPIKey = getEnv("CANYA_SENDGRID_API_KEY", "")
	sendgridTemplate = getEnv("CANYA_SENDGRID_TEMPLATE", "")
	startBlockEnv := getEnvNum("START_BLOCK")
	startBlock = &startBlockEnv
	minutes = getEnvNum("MINUTES") + 1
	addressString := getEnv("ADDRESSES", "")
	err := json.Unmarshal([]byte(addressString), &addresses)
	if err != nil {
		panic(fmt.Sprintf("Error unmarshalling addresses %v", err))
	}
	http.HandleFunc("/", handleRoot)
}

func main() {
	appengine.Main()
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	log.Infof(ctx, "Method start: handleRoot")

	ctxDeadline, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	sendgrid.DefaultClient.HTTPClient = urlfetch.Client(ctxDeadline)
	client := etherscan.NewG(etherscan.Mainnet, etherscanAPIKey, urlfetch.Client(ctxDeadline))

	timestamp := time.Now().UTC()
	timestamp = timestamp.Add(time.Duration(-minutes) * time.Minute)
	log.Infof(ctx, fmt.Sprintf("Checking against time: %s", timestamp.String()))

	for i := 0; i < len(addresses); i++ {
		log.Infof(ctx, fmt.Sprintf("Checking address: %s", addresses[i].Name))
		txs, err := client.NormalTxByAddress(addresses[i].Addr, startBlock, nil, 1, 0, true)
		if err != nil {
			log.Criticalf(ctx, fmt.Sprintf("Error retrieving txs from address %v", addresses[i].Name))
			log.Criticalf(ctx, fmt.Sprintf("%v", err))
			continue
		}
		log.Infof(ctx, fmt.Sprintf("Txs retrieved for address: %s : %d", addresses[i].Name, len(txs)))
		r := funk.Filter(txs, func(tx etherscan.NormalTx) bool {
			log.Debugf(ctx, fmt.Sprintf("Compare: txHash: %s\tatTime: %v\tagainst Time: %v", tx.Hash, tx.TimeStamp.Time(), timestamp))
			return tx.TimeStamp.Time().Unix() > timestamp.Unix() && !addressIsWhitelisted(tx.From)
		})
		if len(r.([]etherscan.NormalTx)) > 0 {
			log.Infof(ctx, fmt.Sprintf("New transactions for address: %s\t count: %d", addresses[i].Name, len(r.([]etherscan.NormalTx))))
			for _, tx := range r.([]etherscan.NormalTx) {
				success := sendEmail(ctx, w, tx, addresses[i])
				if success {
					log.Infof(ctx, fmt.Sprintf("Email sent successfully for tx: %s", tx.Hash))
				} else {
					log.Errorf(ctx, fmt.Sprintf("Email sending error for tx: %s", tx.Hash))
				}
			}
		} else {
			log.Infof(ctx, fmt.Sprintf("No new transactions for address: %s", addresses[i].Name))
		}
	}
}

func addressIsWhitelisted(sender string) (whitelisted bool) {
	for i := 0; i < len(addresses); i++ {
		if strings.ToUpper(addresses[i].Addr) == strings.ToUpper(sender) {
			return true
		}
	}

	return false
}

func sendEmail(ctx context.Context, w http.ResponseWriter, tx etherscan.NormalTx, address address) bool {
	m := mail.NewV3Mail()

	senderAddress := "support@canya.com"
	senderName := "CanYa financial support"
	e := mail.NewEmail(senderName, senderAddress)
	m.SetFrom(e)

	m.SetTemplateID(sendgridTemplate)

	p := mail.NewPersonalization()
	p.AddTos(mail.NewEmail("Allen", "allen@canya.com"))

	log.Infof(ctx, "Sending email...")

	f := new(big.Float).SetInt(tx.Value.Int())
	x := new(big.Float).Quo(f, big.NewFloat(math.Pow(10, 18)))

	txTime := tx.TimeStamp.Time()
	location, err := time.LoadLocation("Australia/Sydney")
	if err == nil {
		txTime = txTime.In(location)
	}

	p.SetDynamicTemplateData("subject", "You have new incoming ethereum transaction")
	p.SetDynamicTemplateData("title", "W00t... someone is sending you ETHOLA!")
	p.SetDynamicTemplateData("amount", fmt.Sprintf("%s ETH", x.String()))
	p.SetDynamicTemplateData("address", fmt.Sprintf("%s - %s", address.Addr, address.Name))
	p.SetDynamicTemplateData("time", fmt.Sprintf("%s", txTime.String()))
	p.SetDynamicTemplateData("returnLinkText", "View transaction")
	p.SetDynamicTemplateData("returnLinkUrl", fmt.Sprintf("https://etherscan.com/tx/%s", tx.Hash))

	m.AddPersonalizations(p)

	request := sendgrid.GetRequest(sendgridAPIKey, "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	var Body = mail.GetRequestBody(m)
	request.Body = Body
	response, err := sendgrid.API(request)
	if err != nil {
		log.Errorf(ctx, "%s", err)
		return false
	} else {
		log.Infof(ctx, "API response status code: %d", response.StatusCode)
		log.Debugf(ctx, "API response body: %s", response.Body)
		log.Debugf(ctx, "API response headers: %+v", response.Headers)
		fmt.Fprintln(w, fmt.Sprintf("email sent with sendgrid response body: %s", response.Body))
		return true
	}
}
