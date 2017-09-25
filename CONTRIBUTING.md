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
    - The requirements are very light, just create a database and a user that has access to it (or use root)
    - Configure .env with connection details
    - Migrations are handled on start up, and are stored in the migrations directory
    - To migrate down, run `gopherci down`
- Ability to accept HTTP/HTTPS requests from GitHub (such as existing public facing server, ngrok, etc)

The following interfaces have alternative implementations that can be used
instead of the default:

- Queue
    - GCPPubSub: a Google Service Account with at least the `PubSub Admin` role, ensure
        `GOOGLE_APPLICATION_CREDENTIALS=file.json` is set.
- Analyser
    - Docker: Running Docker daemon and image `gopherci/gopherci-env:latest` pulled.

# Test GitHub Integration

- Register a new GitHub App with the following:
    - Homepage URL: https://example.com/subdir/
    - Callback URL: https://example.com/subdir/gh/callback
    - Webhook URL: https://example.com/subdir/gh/webhook
    - Webhook secret: shared secret
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
- Install the GitHub App on your account, limiting its permissions to a test repository
- GopherCI should then receive the integration installation event
- Manually enable to repository in the database `UPDATE gh_installations SET enabled_at = NOW() WHERE installation_id = <id>`
- Create a PR or push event on a repository that has the test integration installed
- GopherCI should then receive and process the event

# Integration Tests

Integration tests can be ran using the `make test-integration` command. This requires a series of environment
variables set, see `.env.example` for a list and description. A GitHub account is also required, along with a repository
and personal access token, for this reason it's recommended to use a test account to avoid the issues and comments being
added to your profile, and to ensure the personal access token has minimal access.

Additionally GCPPubSub integration tests require a Google Service Account, see Development Environment.

# Adding a new tool

GopherCI currently (until https://github.com/bradleyfalzon/gopherci/issues/8 is resolved) runs all tools configured
in the database. To add a new tool:

- Choose a tool that:
    - has low false positives, and
    - issues raised are real problems, and
    - outputs in the format: `filename.go:line:colum issue`
- Configure a development environment with the Docker analyser
- Clone https://github.com/gopherci/gopherci-env
- Add the tool to the Dockerfile and build a new image as per repo instructions
- Add the tool to the GopherCI migrations https://github.com/bradleyfalzon/gopherci/tree/master/migrations
- Start GopherCI, the migrations will automatically run
- Create a PR or push event on a repository that has the test integration installed
