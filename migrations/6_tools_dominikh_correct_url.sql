-- +migrate Up
UPDATE tools SET url = "https://github.com/dominikh/go-tools/tree/master/cmd/gosimple" WHERE name = "gosimple";
UPDATE tools SET url = "https://github.com/dominikh/go-tools/tree/master/cmd/staticcheck" WHERE name = "staticcheck";
UPDATE tools SET url = "https://github.com/dominikh/go-tools/tree/master/cmd/unused" WHERE name = "unused";

-- +migrate Down
-- +migrate StatementBegin
-- +migrate StatementEnd
