package router

import "testing"

func TestParamExtraction(t *testing.T) {
	params, ok := matchRoute("/hello/{name}", "/hello/Alice")
	if !ok || params["name"] != "Alice" {
		t.Fatalf("expected name=Alice, got %v", params)
	}

	params2, ok2 := matchRoute("/users/{id}/posts/{postID}", "/users/123/posts/456")
	if !ok2 || params2["id"] != "123" || params2["postID"] != "456" {
		t.Fatalf("expected id=123 postID=456, got %v", params2)
	}
}

func TestInvalidParamRoute(t *testing.T) {
	_, ok := matchRoute("/hello/{name}", "/hello/")
	if ok {
		t.Fatal("expected route not match for /hello/")
	}
	_, ok2 := matchRoute("/users/{id}/posts/{postID}", "/users/123/posts")
	if ok2 {
		t.Fatal("expected route not match for incomplete path")
	}
}
