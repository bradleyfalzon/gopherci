#!/bin/bash -eux

cat > foo2.go <<EOF
package foo
func Foo2() {} // expect golint exported without comment
EOF

git add .
git commit -m "2nd"

git push -f
