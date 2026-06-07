package litepay

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// CallbackBepusdt represents the webhook callback data from BEpusdt.
// BEpusdt sends this JSON payload to the notify_url when payment status changes.
type CallbackBepusdt struct {
	TradeID           string  `json:"trade_id"`
	OrderID           string  `json:"order_id"`
	Amount            float64 `json:"amount"`
	ActualAmount      float64 `json:"actual_amount"`
	Token             string  `json:"token"`
	BlockTransactionID string `json:"block_transaction_id"`
	Signature         string  `json:"signature"`
	Status            int     `json:"status"` // 1=waiting, 2=paid, 3=expired
}

type bepusdt struct {
	Cfg
	apiToken string
	apiURL   string
}

// Bepusdt initializes a BEpusdt USDT cryptocurrency payment provider.
//
// Parameters:
//   - apiToken: Your BEpusdt API token (from admin panel -> System -> API Settings)
//   - apiURL: Your BEpusdt server URL (e.g., "http://your-bepusdt-server:8080")
//
// Returns:
//   - LitePay: A configured BEpusdt payment provider
//
// Supported currencies: CNY, USD, EUR, GBP, JPY
//
// Example:
//
//	pay := litepay.New(callbackURL, successURL, cancelURL)
//	bepusdt := pay.Bepusdt("your_api_token", "http://192.168.1.100:8080")
//	payment, err := bepusdt.Pay(cart)
func (c Cfg) Bepusdt(apiToken, apiURL string) LitePay {
	c.paymentSystem = BEPUSDT
	c.currency = []string{"CNY", "USD", "EUR", "GBP", "JPY"}
	// Remove trailing slash
	apiURL = strings.TrimRight(apiURL, "/")
	return &bepusdt{
		Cfg:      c,
		apiToken: apiToken,
		apiURL:   apiURL,
	}
}

// bepusdtSign generates a BEpusdt-compatible MD5 signature.
//
// Algorithm:
//  1. Sort all non-empty, non-"signature" params by ASCII key order
//  2. Build "key1=value1&key2=value2" string
//  3. Append apiToken (no separator)
//  4. MD5 hash the result, lowercase
func bepusdtSign(data map[string]any, apiToken string) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		if k == "signature" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var signBuf strings.Builder
	for _, k := range keys {
		v := data[k]
		if v == nil || v == "" {
			continue
		}
		signBuf.WriteString(k)
		signBuf.WriteString("=")
		signBuf.WriteString(fmt.Sprintf("%v", v))
		signBuf.WriteString("&")
	}

	signString := strings.TrimRight(signBuf.String(), "&")
	return fmt.Sprintf("%x", md5.Sum([]byte(signString+apiToken)))
}

func (c *bepusdt) Pay(cart Cart) (*Payment, error) {
	currency := strings.ToUpper(cart.Currency)
	if !supportsCurrency(c.currency, currency) {
		return nil, errors.New("this currency is not supported")
	}

	// Calculate total amount in cents, convert to yuan
	var totalCents int
	for _, item := range cart.Items {
		totalCents += item.PriceData.UnitAmount * item.Quantity
	}
	// mycart uses cents (smallest unit). BEpusdt uses yuan.
	// For CNY: 100 cents = 1 yuan. For USD: 100 cents = 1 dollar.
	totalAmount := float64(totalCents) / 100.0

	// Build product name from cart items
	var names []string
	for _, item := range cart.Items {
		names = append(names, item.PriceData.Product.Name)
	}
	productName := strings.Join(names, ", ")
	if len(productName) > 100 {
		productName = productName[:100]
	}

	// Build the callback URL with payment system and cart ID
	callbackURL := fmt.Sprintf("%s/?payment_system=%s&cart_id=%s", c.callbackURL, c.paymentSystem, cart.ID)
	successURL := fmt.Sprintf("%s/?payment_system=%s&cart_id=%s", c.successURL, c.paymentSystem, cart.ID)

	// Build request body
	orderID := cart.ID
	reqBody := map[string]any{
		"order_id":     orderID,
		"amount":       totalAmount,
		"fiat":         currency,
		"trade_type":   "usdt.trc20",
		"name":         productName,
		"notify_url":   callbackURL,
		"redirect_url": successURL,
	}

	// Generate signature
	signature := bepusdtSign(reqBody, c.apiToken)
	reqBody["signature"] = signature

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("bepusdt: failed to marshal request: %w", err)
	}

	// Make API request
	apiEndpoint := c.apiURL + "/api/v1/order/create-transaction"
	req, err := http.NewRequest(http.MethodPost, apiEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("bepusdt: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bepusdt: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bepusdt: failed to read response: %w", err)
	}

	var apiResp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       *struct {
			TradeID        string `json:"trade_id"`
			OrderID        string `json:"order_id"`
			Amount         string `json:"amount"`
			ActualAmount   string `json:"actual_amount"`
			Status         string `json:"status"`
			Token          string `json:"token"`
			ExpirationTime int64  `json:"expiration_time"`
			PaymentURL     string `json:"payment_url"`
		} `json:"data,omitempty"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("bepusdt: failed to parse response: %w", err)
	}

	if apiResp.StatusCode != 200 || apiResp.Data == nil {
		return nil, fmt.Errorf("bepusdt: API error (code=%d): %s", apiResp.StatusCode, apiResp.Message)
	}

	// Parse actual amount (crypto)
	actualAmount, _ := strconv.ParseFloat(apiResp.Data.ActualAmount, 64)

	payment := &Payment{
		PaymentSystem: c.paymentSystem,
		MerchantID:    apiResp.Data.TradeID,
		CartID:        cart.ID,
		AmountTotal:   totalCents,
		Currency:      currency,
		Status:        UNPAID,
		URL:           apiResp.Data.PaymentURL,
		Coin: &Coin{
			AmountTotal: actualAmount,
			Currency:    "USDT",
		},
	}

	return payment, nil
}

// Checkout queries the BEpusdt order status using the trade ID.
func (c *bepusdt) Checkout(payment *Payment, session string) (*Payment, error) {
	if session == "" {
		return payment, nil
	}

	reqBody := map[string]any{
		"trade_id": session,
	}
	signature := bepusdtSign(reqBody, c.apiToken)
	reqBody["signature"] = signature

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("bepusdt: failed to marshal checkout request: %w", err)
	}

	apiEndpoint := c.apiURL + "/api/v1/order/query"
	req, err := http.NewRequest(http.MethodPost, apiEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("bepusdt: failed to create checkout request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		// If query fails, just return current payment state
		return payment, nil
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return payment, nil
	}

	var queryResp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       *struct {
			Status string `json:"status"`
		} `json:"data,omitempty"`
	}
	if err := json.Unmarshal(respBody, &queryResp); err != nil {
		return payment, nil
	}

	if queryResp.Data != nil {
		payment.Status = StatusPayment(c.paymentSystem, queryResp.Data.Status)
	}

	return payment, nil
}
