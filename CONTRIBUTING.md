# Contributing

- Raise an issue first before submitting an PR if you're proposing a new feature.
- Bug fixes are fine to go straight to PR.
- Tests are encouraged, but help can be given to add them if you require.

# Skipping Tests

Some tests require external integration, such as Google Cloud credentials, you
can skip these tests using `go test -short`, and let the CI process run these
during the Pull Request CI tests.

# Development Environment

You'll need:

- Go workspace
- MySQL server 
    - Configure .env with connection details
    - Migrations are handled on start up, and are stored in the migrations directory
    - To migrate down, run `gopherci down`
- Web server with public access (or via ngrok)

The following interfaces have alternative implementations that can be used
instead of the default:

- Queue
    - GCPPubSub: a Google Service Account with at least the `PubSub Admin` role, ensure
        `GOOGLE_APPLICATION_CREDENTIALS=file.json` is set.
- Analyser
    - Docker: Running Docker daemon and image `bradleyfalzon/gopherci-env:latest` pulled.

# Test GitHub Integration

- Register a new GitHub Integration with the following:
    - Homepage URL: https://example.com/subdir/
    - Callback URL: https://example.com/subdir/gh/callback
    - Web hook URL: https://example.com/subdir/gh/webhook
    - Web hook secret: shared secret
    - Permissions
        - Repository metadata: Read-only
        - Commit statuses: Read & Write
        - Repository contents: Read-only
        - Pull requests: Read & write
            - Pull request event
    - Installed on: Only on this account
- Once you've registered the integration
	- Generate private key, save it somewhere accessible to GopherCI and set the .env file or environment
	- Record the integration id in the .env file or environment
- Start GopherCI
- Install the GitHub integration
- GopherCI should then receive the web hook
- Create a test repo
