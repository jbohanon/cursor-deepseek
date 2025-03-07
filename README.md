# OpenAI API Proxy

A high-performance HTTP/2-enabled proxy server designed to interface the OpenAI API specification to alternative language models. 

This proxy translates OpenAI-compatible API requests to alternative API formats, allowing Cursor's Composer and other OpenAI API-compatible tools to seamlessly work with these models.

## Primary Use Case

This proxy was created originally to enable Cursor IDE users to leverage alternative (e.g. DeepSeek, OpenRouter, and Ollama) powerful language models through Cursor's Composer interface as an alternative to OpenAI's models. By running this proxy locally, you can configure Cursor's Composer to use these models for AI assistance, code generation, and other AI features. It handles all the necessary request/response translations and format conversions to make the integration seamless.

## Features

- HTTP/2 support for improved performance
- Full CORS support
- Streaming responses
- Support for function calling/tools
- Automatic message format conversion
- Compression support (Brotli, Gzip, Deflate) (DeepSeek only)
- Compatible with OpenAI API client libraries
- API key validation for secure access
- ~~Docker container support with multi-variant builds~~ returning soon

## Prerequisites for Cursor use

- Cursor Pro Subscription
- Go 1.24 or higher
- DeepSeek or OpenRouter API key
- Ollama server running locally (optional, for Ollama support)
- Public Endpoint

## Installation

1. Clone the repository
1. Install dependencies:
```bash
go mod download
```

<!--- TODO: fix docker
### Docker Installation

The proxy supports both DeepSeek and OpenRouter variants. Choose the appropriate build command for your needs:

1. Build the Docker image:
   - For DeepSeek (default):
   ```bash
   docker build -t cursor-deepseek .
   ```
   - For OpenRouter:
   ```bash
   docker build -t cursor-openrouter --build-arg PROXY_VARIANT=openrouter .
   ```
   - For Ollama:
   ```bash
   docker build -t cursor-ollama --build-arg PROXY_VARIANT=ollama .
   ```

2. Configure environment variables:
   - Copy the example configuration:
   ```bash
   cp .env.example .env
   ```
   - Edit `.env` and add your API key (either DeepSeek or OpenRouter)

3. Run the container:
```bash
docker run -p 9000:9000 --env-file .env cursor-deepseek
# OR for OpenRouter
docker run -p 9000:9000 --env-file .env cursor-openrouter
# OR for Ollama
docker run -p 9000:9000 --env-file .env cursor-ollama
```
--->

## Configuration


Backend configuration will be selected based on precedent of configured values as below:
1. If config.yaml `deepseek.api_key` or env `DEEPSEEK_API_KEY` is set, the DeepSeek backend will be used.
1. If config.yaml `openrouter.api_key` or env `OPENROUTER_API_KEY` is set, the OpenRouter backend will be used.
1. If config.yaml `ollama.endpoint` or env `OLLAMA_ENDPOINT` is set, the Ollama backend will be used.

```yaml
port: "9000"
log_level: info # one of trace, debug, info, warn, error, fatal
timeout: 60s # must be duration format compatible with Go's duration parsing. Read about it [here](https://pkg.go.dev/time#ParseDuration)

# note that only one backend should be configured, but they all have the same options
# deepseek:
# openrouter:
ollama:
  endpoint: "http://127.0.0.1:11434/api"
  api_key: "foobar"
  models:
    o1: deepseek-r1:14b
    gpt-4o: llama3
    gpt-3.5-turbo: llama2
    claude-3.7-sonnet: deepseek-r1:14b
  default_model: llama3
```

## Usage

1. Start by copying the config.yaml.example to config.yaml `cp ./config.yaml.example ./config.yaml`
1. Add config per the options.
1. Run the proxy with `go run ./cmd/main.go`
1. Use the proxy with your OpenAI API clients by setting the base URL to `http://your-public-endpoint:9000/v1`

## Config Reference

## Exposing the Endpoint Publicly

You can expose your local proxy server to the internet using ngrok or similar services. This is useful when you need to access the proxy from external applications or different networks.

The methods listed herein are for reference only and should _NOT_ be used for production services. For such, a reverse proxy or API gateway would be advisable.

### `ngrok`
1. Install ngrok from https://ngrok.com/download
1. Start your proxy server locally (it will run on port 9000)
1. In a new terminal, run ngrok: `ngrok http 9000`
1. ngrok will provide you with a public URL (e.g., https://your-unique-id.ngrok.io)
1. Use this URL as your OpenAI API base URL in Cursor's settings: `https://your-unique-id.ngrok.io/v1`

### Cloudflare Tunnel
1. Install cloudflared (see https://github.com/cloudflare/cloudflared for details)
1. Run: `cloudflared tunnel --url http://localhost:9000`

### LocalTunnel
1. Install: `npm install -g localtunnel`
2. Run: `lt --port 9000`

Remember to always secure your endpoint appropriately when exposing it to the internet.


## Supported Endpoints

- `/v1/chat/completions` - Chat completions endpoint
- `/v1/models` - Models listing endpoint

## Model Mapping
Models may be mapped by backend configuration. If no model mapping exists, then all requests will use the configured defaultModel. If _that_ is not configured, then they will use default models defined in `internal/constants/<backend>/<backend>.go`. These defaults are:
- DeepSeek backend: `deepseek-chat`
- OpenRouter backend: `deepseek/deepseek-chat`
- Ollama backend: `llama3`

## Security

- The proxy includes CORS headers for cross-origin requests
- API keys are required and validated against config or, as a fallback, environment variables
- Secure handling of request/response data
- Strict API key validation for all requests (if API key is configured) <!-- TODO: validate API Key is configured for DS and OR backends -->
- HTTPS support
- `config.yaml` and `.env` are never committed to the repository

## License

This project is licensed under the GNU General Public License v2.0 (GPLv2). See the [LICENSE.md](LICENSE.md) file for details.
