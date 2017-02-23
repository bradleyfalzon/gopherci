#!/bin/bash -eux

git checkout -b issue-comments

cat > foo.go <<EOF
package foo
func Foo() {}  // expect golint exported without comment
EOF

git add .
git commit -m "commit"

git push -f -u origin issue-comments
