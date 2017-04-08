package web

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/db"
)

func TestAnalysisFiles(t *testing.T) {
	diffReader := bytes.NewBuffer([]byte(`diff --git a/main.go b/main.go
index 4810940..4090359 100644
--- a/main.go
+++ b/main.go
@@ -3,5 +3,5 @@ package main
 import "fmt"
 
 func main() {
-       fmt.Println("Hi")
+       fmt.Println("Hi: %v", "alice")
 }
diff --git a/noissues.go b/noissues.go
new file mode 100644
index 0000000..3de84a3
--- /dev/null
+++ b/noissues.go
@@ -0,0 +1,3 @@
+package main
+
+func foo() {}
`))

	// TODO give it more issues
	issues := []db.Issue{
		{Path: "main.go", Line: 6, Issue: "issue here"},
	}

	havePatches, err := DiffIssues(context.Background(), diffReader, issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPatches := []Patch{{
		Path: "main.go",
		Hunks: []Hunk{
			{
				Range: "@@ -3,5 +3,5 @@",
				Lines: []Line{
					{Line: "import \"fmt\"", ChangeType: "none", LineNo: 3, Issues: nil},
					{Line: "", ChangeType: "none", LineNo: 4, Issues: nil},
					{Line: "func main() {", ChangeType: "none", LineNo: 5, Issues: nil},
					{Line: "       fmt.Println(\"Hi\")", ChangeType: "remove", LineNo: 6, Issues: nil},
					{Line: "       fmt.Println(\"Hi: %v\", \"alice\")", ChangeType: "add", LineNo: 6, Issues: []db.Issue{
						{Path: "main.go", Line: 6, Issue: "issue here"}},
					},
					{Line: "}", ChangeType: "none", LineNo: 7, Issues: nil},
				},
			},
		},
	}}

	if !reflect.DeepEqual(havePatches, wantPatches) {
		t.Errorf("\nhave: %#v\nwant: %#v", havePatches, wantPatches)
	}
}
