#!/bin/bash -eux

git checkout -b $1

mkdir cgo
cat > cgo/main.go <<EOF
package main

/*
#include <gsl/gsl_version.h>

const char* ver() {
        return GSL_VERSION;
    }
*/
import "C"

func main() {
        println(C.GoString(C.ver()))
}
EOF

cat > .gopherci.yml <<EOF
apt_packages:
    - libgsl0-dev
EOF

git add .
git commit -m "commit"
git push -f -u origin HEAD
