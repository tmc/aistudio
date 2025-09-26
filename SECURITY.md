# Security Best Practices for AIStudio

## API Key Management

### Environment Variables
- **Recommended**: Store API keys in environment variables
  ```bash
  export GEMINI_API_KEY="your-api-key-here"
  export GROK_API_KEY="your-grok-api-key-here"
  ```
- Never hardcode API keys directly in source code
- Never commit API keys to version control

### Application Default Credentials (ADC)
- For Google Cloud environments, use ADC when possible:
  ```bash
  gcloud auth application-default login
  ```
- ADC provides automatic key rotation and better security

### Command-Line Arguments
- While `--api-key` flag is available, avoid using it in shared environments
- Command-line arguments may be visible in process lists
- Prefer environment variables or ADC

## Secure Storage

### History Files
- Chat history is stored locally in `./history` directory by default
- Contains conversation data that may include sensitive information
- Set appropriate file permissions:
  ```bash
  chmod 700 ./history
  ```
- Consider encrypting the history directory on sensitive systems

### Configuration Files
- Tool definition files may contain sensitive data
- Store configuration files with restrictive permissions:
  ```bash
  chmod 600 config.json
  ```

## Network Security

### TLS/HTTPS
- All API communications use HTTPS by default
- gRPC connections are encrypted with TLS
- WebSocket connections upgrade from HTTPS

### Proxy Support
- Respects standard proxy environment variables:
  - `HTTPS_PROXY`
  - `HTTP_PROXY`
  - `NO_PROXY`

## Tool Approval Security

### Enable Tool Approval
- Use `--require-approval` flag for untrusted environments
- Review each tool call before execution
- Auto-approve only trusted tools with Ctrl+A toggle

### Tool Restrictions
- Be cautious with tools that:
  - Execute system commands
  - Access filesystem
  - Make network requests
  - Handle sensitive data

## MCP Security

### Server Mode
- Bind to localhost only by default
- Use authentication when exposing to network:
  ```bash
  aistudio --mcp-server --port 8080 --bind 127.0.0.1
  ```

### Client Connections
- Verify MCP server certificates
- Use encrypted transports (HTTPS/WSS) for remote servers
- Validate tool capabilities before enabling

## Voice & Video Privacy

### Voice Input
- Audio is processed locally before transmission
- Voice activity detection prevents unnecessary streaming
- Mute functionality available with keyboard shortcuts

### Video/Camera Input
- Camera and screen capture require explicit user consent
- Privacy mode disables all video inputs
- Frame data is not stored unless explicitly configured

### Data Retention
- Streamed audio/video is not retained by default
- Recording features require explicit opt-in
- Clear recordings after use

## Audit & Logging

### Sensitive Data in Logs
- Production logs should not contain:
  - API keys or tokens
  - Personal conversation content
  - Tool execution results with sensitive data

### Debug Mode
- Use `--debug` only in development
- Debug logs may contain sensitive information
- Disable debug logging in production

## Recommendations

1. **Principle of Least Privilege**
   - Grant minimum necessary permissions
   - Use read-only access where possible
   - Limit tool capabilities

2. **Regular Updates**
   - Keep dependencies updated
   - Monitor security advisories
   - Update Go runtime regularly

3. **Environment Isolation**
   - Use separate API keys for dev/staging/production
   - Isolate sensitive operations
   - Use containers or VMs for additional isolation

4. **Monitoring**
   - Monitor API usage for anomalies
   - Set up alerts for unusual activity
   - Review tool execution logs

5. **Incident Response**
   - Rotate API keys immediately if compromised
   - Review history files for sensitive data exposure
   - Document security incidents

## Reporting Security Issues

If you discover a security vulnerability:
1. Do NOT create a public GitHub issue
2. Email security concerns to the maintainer
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if available)

## Compliance Notes

- This software processes data through third-party APIs
- Ensure compliance with your organization's data policies
- Review Terms of Service for API providers
- Consider data residency requirements