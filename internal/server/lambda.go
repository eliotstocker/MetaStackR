package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
)

type LambdaHandler struct {
	adapter *httpadapter.HandlerAdapter
}

func NewLambdaHandler(handler http.Handler) *LambdaHandler {
	return &LambdaHandler{
		adapter: httpadapter.New(handler),
	}
}

func (h *LambdaHandler) Proxy(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	// Translate V2 event to V1 internally for the adapter
	v1Req := events.APIGatewayProxyRequest{
		Path:                  req.RawPath,
		HTTPMethod:            req.RequestContext.HTTP.Method,
		Headers:               req.Headers,
		QueryStringParameters: req.QueryStringParameters,
		Body:                  req.Body,
		IsBase64Encoded:       req.IsBase64Encoded,
	}

	v1Resp, err := h.adapter.ProxyWithContext(ctx, v1Req)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, err
	}

	headers := make(map[string]string)
	for k, v := range v1Resp.Headers {
		headers[k] = v
	}
	for k, vals := range v1Resp.MultiValueHeaders {
		if len(vals) > 0 {
			headers[k] = strings.Join(vals, ", ")
		}
	}

	headers["Access-Control-Allow-Origin"] = "*"
	headers["Access-Control-Allow-Methods"] = "GET, POST, PUT, DELETE, OPTIONS"
	headers["Access-Control-Allow-Headers"] = "Content-Type, Authorization, X-Requested-With"

	return events.APIGatewayV2HTTPResponse{
		StatusCode:      v1Resp.StatusCode,
		Headers:         headers,
		Body:            v1Resp.Body,
		IsBase64Encoded: v1Resp.IsBase64Encoded,
	}, nil
}

func StartLambda(handler http.Handler) {
	h := NewLambdaHandler(handler)
	lambda.Start(h.Proxy)
}
