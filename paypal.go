package paypal

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	NVP_SANDBOX_URL         = "https://api-3t.sandbox.paypal.com/nvp"
	NVP_PRODUCTION_URL      = "https://api-3t.paypal.com/nvp"
	CHECKOUT_SANDBOX_URL    = "https://www.sandbox.paypal.com/cgi-bin/webscr"
	CHECKOUT_PRODUCTION_URL = "https://www.paypal.com/cgi-bin/webscr"
	NVP_VERSION             = "94"
)

type PayPalClient struct {
	username    string
	password    string
	signature   string
	usesSandbox bool
	client      *http.Client
}

type PayPalOrder struct {
	SubTotal     float64
	Shipping     float64
	Discount     float64
	Total        float64
	CurrencyCode string
	ReturnUrl    string
	CancelUrl    string
}

type PayPalDigitalGood struct {
	Name     string
	Amount   float64
	Quantity int
}

type PayPalGood struct {
	Id       string
	Name     string
	Amount   float64
	Quantity int
}

type PayPalResponse struct {
	Ack           string
	CorrelationId string
	Timestamp     string
	Version       string
	Build         string
	Token         string
	Values        url.Values
	usedSandbox   bool
}

type PayPalPaymentResponse struct {
	TransactionId string
	Status        string
	Type          string
	Fee           float64
	Amount        float64
	Currency      string
	ReasonCode    string
}

type PayPalError struct {
	Ack          string
	ErrorCode    string
	ShortMessage string
	LongMessage  string
	SeverityCode string
}

func (e *PayPalError) Error() string {
	var message string
	if len(e.ErrorCode) != 0 && len(e.ShortMessage) != 0 {
		message = "PayPal Error " + e.ErrorCode + ": " + e.ShortMessage
	} else if len(e.Ack) != 0 {
		message = e.Ack
	} else {
		message = "PayPal is undergoing maintenance.\nPlease try again later."
	}

	return message
}

func (r *PayPalResponse) CheckoutUrl() string {
	query := url.Values{}
	query.Set("cmd", "_express-checkout")
	query.Add("token", r.Token)
	checkoutUrl := CHECKOUT_PRODUCTION_URL
	if r.usedSandbox {
		checkoutUrl = CHECKOUT_SANDBOX_URL
	}
	return fmt.Sprintf("%s?%s", checkoutUrl, query.Encode())
}

func SumPayPalDigitalGoodAmounts(goods *[]PayPalDigitalGood) (sum float64) {
	for _, dg := range *goods {
		sum += dg.Amount * float64(dg.Quantity)
	}
	return
}

func NewDefaultClient(username, password, signature string, usesSandbox bool) *PayPalClient {
	return &PayPalClient{username, password, signature, usesSandbox, new(http.Client)}
}

func NewClient(username, password, signature string, usesSandbox bool, client *http.Client) *PayPalClient {
	return &PayPalClient{username, password, signature, usesSandbox, client}
}

func (pClient *PayPalClient) PerformRequest(values url.Values) (*PayPalResponse, error) {
	values.Add("USER", pClient.username)
	values.Add("PWD", pClient.password)
	values.Add("SIGNATURE", pClient.signature)
	values.Add("VERSION", NVP_VERSION)

	endpoint := NVP_PRODUCTION_URL
	if pClient.usesSandbox {
		endpoint = NVP_SANDBOX_URL
	}

	formResponse, err := pClient.client.PostForm(endpoint, values)
	if err != nil {
		return nil, err
	}
	defer formResponse.Body.Close()

	body, err := ioutil.ReadAll(formResponse.Body)
	if err != nil {
		return nil, err
	}

	responseValues, err := url.ParseQuery(string(body))
	response := &PayPalResponse{usedSandbox: pClient.usesSandbox}
	if err == nil {
		response.Ack = responseValues.Get("ACK")
		response.CorrelationId = responseValues.Get("CORRELATIONID")
		response.Timestamp = responseValues.Get("TIMESTAMP")
		response.Version = responseValues.Get("VERSION")
		response.Build = responseValues.Get("BUILD")
		response.Token = responseValues.Get("TOKEN")
		response.Values = responseValues

		errorCode := responseValues.Get("L_ERRORCODE0")
		if len(errorCode) != 0 || strings.ToLower(response.Ack) == "failure" || strings.ToLower(response.Ack) == "failurewithwarning" {
			pError := new(PayPalError)
			pError.Ack = response.Ack
			pError.ErrorCode = errorCode
			pError.ShortMessage = responseValues.Get("L_SHORTMESSAGE0")
			pError.LongMessage = responseValues.Get("L_LONGMESSAGE0")
			pError.SeverityCode = responseValues.Get("L_SEVERITYCODE0")

			err = pError
		}
	}

	return response, err
}

func (response *PayPalPaymentResponse) Populate(values url.Values) {
	response.TransactionId = values.Get("PAYMENTINFO_0_TRANSACTIONID")
	response.Status = values.Get("PAYMENTINFO_0_PAYMENTSTATUS")
	paymentAmt := values.Get("PAYMENTINFO_0_AMT")
	response.Amount, _ = strconv.ParseFloat(paymentAmt, 10)
	feeAmt := values.Get("PAYMENTINFO_0_FEEAMT")
	response.Fee, _ = strconv.ParseFloat(feeAmt, 10)
	response.Currency = values.Get("PAYMENTINFO_0_CURRENCYCODE")
	response.Type = values.Get("PAYMENTINFO_0_PAYMENTTYPE")
	response.ReasonCode = values.Get("PAYMENTINFO_0_REASONCODE")
}

func (pClient *PayPalClient) SetExpressCheckoutDigitalGoods(paymentAmount float64, currencyCode string, returnURL, cancelURL string, goods []PayPalDigitalGood) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "SetExpressCheckout")
	values.Add("PAYMENTREQUEST_0_AMT", fmt.Sprintf("%.2f", paymentAmount))
	values.Add("PAYMENTREQUEST_0_PAYMENTACTION", "Sale")
	values.Add("PAYMENTREQUEST_0_CURRENCYCODE", currencyCode)
	values.Add("RETURNURL", returnURL)
	values.Add("CANCELURL", cancelURL)
	values.Add("REQCONFIRMSHIPPING", "0")
	values.Add("NOSHIPPING", "1")
	values.Add("SOLUTIONTYPE", "Sole")

	for i := 0; i < len(goods); i++ {
		good := goods[i]

		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_NAME", i), good.Name)
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_AMT", i), fmt.Sprintf("%.2f", good.Amount))
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_QTY", i), fmt.Sprintf("%d", good.Quantity))
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_ITEMCATEGORY", i), "Digital")
	}

	return pClient.PerformRequest(values)
}

func (pClient *PayPalClient) SetExpressCheckout(order PayPalOrder, goods []PayPalGood) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "SetExpressCheckout")
	values.Add("PAYMENTREQUEST_0_ITEMAMT", fmt.Sprintf("%.2f", order.SubTotal))
	values.Add("PAYMENTREQUEST_0_SHIPPINGAMT", fmt.Sprintf("%.2f", order.Shipping))
	values.Add("PAYMENTREQUEST_0_AMT", fmt.Sprintf("%.2f", order.Total))
	values.Add("PAYMENTREQUEST_0_PAYMENTACTION", "Sale")
	values.Add("PAYMENTREQUEST_0_CURRENCYCODE", order.CurrencyCode)
	values.Add("RETURNURL", order.ReturnUrl)
	values.Add("CANCELURL", order.CancelUrl)
	values.Add("REQCONFIRMSHIPPING", "0")
	values.Add("NOSHIPPING", "1")
	values.Add("SOLUTIONTYPE", "Sole")

	goodsCount := len(goods)

	for i := 0; i < goodsCount; i++ {
		good := goods[i]
		if good.Id != "" {
			values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_NUMBER", i), good.Id)
		}
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_NAME", i), good.Name)
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_AMT", i), fmt.Sprintf("%.2f", good.Amount))
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_QTY", i), fmt.Sprintf("%d", good.Quantity))
	}

	if order.Discount > 0 {
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_NAME", goodsCount), "DISCOUNT")
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_AMT", goodsCount), fmt.Sprintf("%.2f", -order.Discount))
		values.Add(fmt.Sprintf("%s%d", "L_PAYMENTREQUEST_0_QTY", goodsCount), "1")
	}

	return pClient.PerformRequest(values)
}

// Convenience function for Sale (Charge)
func (pClient *PayPalClient) DoExpressCheckoutSale(token, payerId, currencyCode string, finalPaymentAmount float64) (*PayPalResponse, error) {
	return pClient.DoExpressCheckoutPayment(token, payerId, "Sale", currencyCode, finalPaymentAmount)
}

// paymentType can be "Sale" or "Authorization" or "Order" (ship later)
func (pClient *PayPalClient) DoExpressCheckoutPayment(token, payerId, paymentType, currencyCode string, finalPaymentAmount float64) (*PayPalResponse, error) {
	values := url.Values{}
	values.Set("METHOD", "DoExpressCheckoutPayment")
	values.Add("TOKEN", token)
	values.Add("PAYERID", payerId)
	values.Add("PAYMENTREQUEST_0_PAYMENTACTION", paymentType)
	values.Add("PAYMENTREQUEST_0_CURRENCYCODE", currencyCode)
	values.Add("PAYMENTREQUEST_0_AMT", fmt.Sprintf("%.2f", finalPaymentAmount))

	return pClient.PerformRequest(values)
}

func (pClient *PayPalClient) GetExpressCheckoutDetails(token string) (*PayPalResponse, error) {
	values := url.Values{}
	values.Add("TOKEN", token)
	values.Set("METHOD", "GetExpressCheckoutDetails")
	return pClient.PerformRequest(values)
}
