# Testing checklist

Run these first:

```powershell
go mod tidy
go test ./...
go vet ./...
go build ./...
```

Then test on a VPS or local development mode.

## HTTP

1. Start a local HTTP service on 8000.
2. Create a tunnel user.
3. Run the client.
4. Open the printed public URL.

## Reconnect

Stop the client, wait for the public endpoint to become unavailable, then start
the client again.

## Account enforcement

Create a short-lived test account and verify expiry disconnects it.

Set a very small bandwidth limit and transfer data through the tunnel.

## TCP

Run a local TCP service and create a TCP-enabled account. Connect to the assigned
public TCP port.

## Security

Verify port 9800 is not exposed publicly. Verify the admin dashboard requires
authentication. Verify an old token stops working after a token reset.
