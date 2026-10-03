package tools

import "testing"

func TestRecountHunkHeaders(t *testing.T) {
	t.Parallel()
	in := "@@ -1,2 +1,2 @@ gamma\n alpha\n beta\n-gamma\n+GAMMA\n@@ -8,1 +8,1 @@\n-old\n+new\n"
	got := recountHunkHeaders(in)
	want := "@@ -1,3 +1,3 @@ gamma\n alpha\n beta\n-gamma\n+GAMMA\n@@ -8,1 +8,1 @@\n-old\n+new\n"
	if got != want {
		t.Fatalf("recount = %q", got)
	}
	// A non-body line means the header is left for the parser to reject.
	dirty := "@@ -1,5 +1,5 @@\n-a\n+b\nnot a hunk line\n"
	if recountHunkHeaders(dirty) != dirty {
		t.Fatalf("dirty hunk was rewritten: %q", recountHunkHeaders(dirty))
	}
	noEOL := "@@ -1,9 +1,9 @@\n alpha\n-beta\n\\ No newline at end of file\n+BETA\n\\ No newline at end of file\n"
	wantEOL := "@@ -1,2 +1,2 @@\n alpha\n-beta\n\\ No newline at end of file\n+BETA\n\\ No newline at end of file\n"
	if got := recountHunkHeaders(noEOL); got != wantEOL {
		t.Fatalf("no eol recount = %q", got)
	}
}

func TestApplyMiscountedHunk(t *testing.T) {
	t.Parallel()
	content := []byte("alpha\nbeta\ngamma\n")
	// Header says two old lines; the body has three. That is the
	// "fragment contains no changes" failure from a short @@ count.
	next, err := ApplyUnified("f.txt", "@@ -1,2 +1,2 @@\n alpha\n beta\n-gamma\n+GAMMA\n", content)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "alpha\nbeta\nGAMMA\n" {
		t.Fatalf("undercount = %q", next)
	}

	next, err = ApplyUnified("f.txt", "@@ -1,8 +1,8 @@\n alpha\n-beta\n+BETA\n gamma\n", content)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "alpha\nBETA\ngamma\n" {
		t.Fatalf("overcount = %q", next)
	}

	two := []byte("a\nb\nc\nd\n")
	next, err = ApplyUnified("f.txt", "@@ -1,3 +1,3 @@\n-a\n+A\n@@ -4,2 +4,2 @@\n-d\n+D\n", two)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "A\nb\nc\nD\n" {
		t.Fatalf("two hunks = %q", next)
	}

	noEOL := []byte("alpha\nbeta")
	next, err = ApplyUnified("f.txt", "@@ -1,4 +1,4 @@\n alpha\n-beta\n\\ No newline at end of file\n+BETA\n\\ No newline at end of file\n", noEOL)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != "alpha\nBETA" {
		t.Fatalf("no eol = %q", next)
	}

	if _, err := ApplyUnified("f.txt", "@@ -1,1 +1,1 @@\n alpha\n beta\n", content); err == nil {
		t.Fatal("context-only hunk accepted")
	}
	if _, err := ApplyUnified("f.txt", "@@ -1,2 +1,2 @@\n nope\n beta\n-gamma\n+GAMMA\n", content); err == nil {
		t.Fatal("mismatched context accepted")
	}
	if _, err := ApplyUnified("f.txt", "@@ -1,2 +1,2 @@\n-alpha\n+ALPHA\nnot a hunk line\n", []byte("alpha\n")); err == nil {
		t.Fatal("dirty hunk applied")
	}
}
