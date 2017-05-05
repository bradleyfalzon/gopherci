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
    - Docker: Running Docker daemon and image `gopherci/gopherci-env:latest` pulled.

# Test GitHub Integration

- Register a new GitHub Integration with the following:
    - Homepage URL: https://example.com/subdir/
    - Callback URL: https://example.com/subdir/gh/callback
    - Web hook URL: https://example.com/subdir/gh/webhook
    - Web hook secret: shared secret
    - Permissions
        - Repository metadata: Read-only
            - Repository event (remove public information when project change to private or deleted #22)
        - Commit statuses: Read & Write (update the commit status API eg when checking PR)
        - Repository contents: Read-only (clone repository)
            - Push event (check pushes to repository #27)
        - Pull requests: Read & write (write comments)
            - Pull request event (check PRs to repository)
    - Installed on: Only on this account
- Once you've registered the integration
    - Generate private key, save it somewhere accessible to GopherCI and set the .env file or environment
    - Record the integration id in the .env file or environment
- Start GopherCI
- Install the GitHub integration
- GopherCI should then receive the web hook
- Create a test repo

# Integration Tests

Integration tests can be ran using the `make test-integration` command. This requires a series of environment
variables set, see `.env.example` for a list and description. A GitHub account is also required, along with a repository
and personal access token, for this reason it's recommended to use a test account to avoid the issues and comments being
added to your profile, and to ensure the personal access token has minimal access.

Additionally GCPPubSub integration tests require a Google Service Account, see Development Environment.
