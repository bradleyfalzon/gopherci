#!/bin/bash -eux

if [[ "$2" == "step-1" ]]; then

    git checkout -b $1

    cat > foo.go <<EOF
package foo
func Foo() {}  // expect golint exported without comment
EOF

    git add .
    git commit -m "commit"

    git push -u origin HEAD

elif [[ "$2" == "step-2" ]]; then

    git checkout $1

    cat > foo.go <<EOF
package foo
func Bar() {}  // expect golint exported without comment
EOF

    git add .
    git commit -m "commit2"

    git push -f

else

    git checkout $1

    cat > foo2.go <<EOF
package foo
func Foo2() {}  // expect golint exported without comment
EOF

    git add .
    git commit -m "commit3"

    git push
fi
