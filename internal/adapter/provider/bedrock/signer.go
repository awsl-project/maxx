package bedrock

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

const bedrockService = "bedrock"

// signRequest signs an HTTP request with AWS SigV4.
func signRequest(ctx context.Context, req *http.Request, body []byte, creds credentials.StaticCredentialsProvider, region string) error {
	awsCreds, err := creds.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	hash := sha256.Sum256(body)
	payloadHash := fmt.Sprintf("%x", hash)

	signer := v4.NewSigner()
	return signer.SignHTTP(ctx, awsCreds, req, payloadHash, bedrockService, region, time.Now())
}

// buildBedrockURL constructs the Bedrock endpoint URL.
func buildBedrockURL(region, modelID string, stream bool) string {
	base := fmt.Sprintf(BedrockURLTemplate, region)
	if stream {
		return base + fmt.Sprintf(InvokeStreamPath, modelID)
	}
	return base + fmt.Sprintf(InvokePath, modelID)
}
