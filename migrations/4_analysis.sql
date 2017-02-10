-- +migrate Up
CREATE TABLE analysis (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    gh_installation_id INT UNSIGNED NOT NULL,
    repository_id INT UNSIGNED,
    status ENUM("Pending", "Failure", "Success", "Error") DEFAULT "Pending",
    clone_duration TIME(3) NULL DEFAULT NULL,
    deps_duration TIME(3) NULL DEFAULT NULL,
    total_duration TIME(3) NULL DEFAULT NULL,
    created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY (gh_installation_id),
    KEY (repository_id),
    FOREIGN KEY (gh_installation_id) REFERENCES gh_installations(id) ON DELETE CASCADE
);

CREATE TABLE analysis_tool (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    analysis_id INT UNSIGNED NOT NULL,
    tool_id INT UNSIGNED NOT NULL,
    duration TIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY (analysis_id),
    KEY (tool_id),
    FOREIGN KEY (analysis_id) REFERENCES analysis(id) ON DELETE CASCADE,
    FOREIGN KEY (tool_id) REFERENCES tools(id) ON DELETE CASCADE
);

CREATE TABLE issues (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    analysis_tool_id INT UNSIGNED NOT NULL,
    path VARCHAR(255) NOT NULL,
    line INT UNSIGNED NOT NULL,
    hunk_pos INT UNSIGNED NOT NULL,
    issue TEXT,
    PRIMARY KEY (id),
    KEY (analysis_tool_id),
    FOREIGN KEY (analysis_tool_id) REFERENCES analysis_tool(id) ON DELETE CASCADE
);

-- +migrate Down
DROP TABLE issues;
DROP TABLE analysis_tool;
DROP TABLE analysis;
