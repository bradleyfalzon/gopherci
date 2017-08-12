-- +migrate Up
INSERT INTO tools (name, url, path, args, `regexp`) VALUES
    ("unparam", "https://github.com/mvdan/unparam", "unparam", "./...", "");

-- +migrate Down
-- +migrate StatementBegin
-- +migrate StatementEnd
