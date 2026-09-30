package httpapi

import "testing"

// TestAdminSectionAllowed_Matrix: the full matrix of roles × sections × (read/write).
// This is the most important security test: every change to this function must
// explicitly break or confirm specific cells here.
//
// Sections are taken from the implementation: stats, system, users, access,
// dialogs, modes, orchestration, exports, tariffs, billing.
func TestAdminSectionAllowed_Matrix(t *testing.T) {
	t.Parallel()

	// "x" = allowed, "-" = denied. Columns: read | write.
	type rw struct {
		read, write bool
	}
	type section = string
	matrix := map[string]map[section]rw{
		"owner": {
			"stats": {true, true}, "system": {true, true},
			"users": {true, true}, "access": {true, true}, "dialogs": {true, true},
			"modes": {true, true}, "orchestration": {true, true}, "exports": {true, true},
			"tariffs": {true, true}, "billing": {true, true},
		},
		"admin": {
			"stats": {true, true}, "system": {true, true},
			"users": {true, true}, "access": {true, true}, "dialogs": {true, true},
			"modes": {true, true}, "orchestration": {true, true}, "exports": {true, true},
			"tariffs": {true, true}, "billing": {true, true},
		},
		"billing_admin": {
			"stats": {true, false}, "system": {true, false},
			"users": {false, false}, "access": {false, false}, "dialogs": {false, false},
			"modes": {false, false}, "orchestration": {false, false}, "exports": {false, false},
			"tariffs": {true, false}, "billing": {true, false},
		},
		"content_admin": {
			"stats": {false, false}, "system": {false, false},
			"users": {false, false}, "access": {false, false}, "dialogs": {false, false},
			"modes": {true, false}, "orchestration": {true, false}, "exports": {false, false},
			"tariffs": {false, false}, "billing": {false, false},
		},
		"support": {
			"stats": {true, false}, "system": {true, false},
			"users": {true, false}, "access": {true, false}, "dialogs": {true, false},
			"modes": {false, false}, "orchestration": {false, false}, "exports": {true, false},
			"tariffs": {false, false}, "billing": {false, false},
		},
		"tester": {
			"stats": {false, false}, "system": {false, false},
			"users": {false, false}, "access": {false, false}, "dialogs": {false, false},
			"modes": {false, false}, "orchestration": {false, false}, "exports": {false, false},
			"tariffs": {false, false}, "billing": {false, false},
		},
		"user": {
			"stats": {false, false}, "system": {false, false},
			"users": {false, false}, "access": {false, false}, "dialogs": {false, false},
			"modes": {false, false}, "orchestration": {false, false}, "exports": {false, false},
			"tariffs": {false, false}, "billing": {false, false},
		},
	}
	for role, sections := range matrix {
		for sec, want := range sections {
			if got := adminSectionAllowed(role, sec, false); got != want.read {
				t.Errorf("read [%s][%s] = %v, want %v", role, sec, got, want.read)
			}
			if got := adminSectionAllowed(role, sec, true); got != want.write {
				t.Errorf("write [%s][%s] = %v, want %v", role, sec, got, want.write)
			}
		}
	}
}

// TestAdminSectionAllowed_UnknownSection: an unknown section → always false (except owner/admin).
func TestAdminSectionAllowed_UnknownSection(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"billing_admin", "content_admin", "support", "tester", "user"} {
		if adminSectionAllowed(role, "fictional_section", false) {
			t.Errorf("role %q got read access to fictional section", role)
		}
		if adminSectionAllowed(role, "fictional_section", true) {
			t.Errorf("role %q got write access to fictional section", role)
		}
	}
	// owner/admin pass anyway (full bypass)
	if !adminSectionAllowed("owner", "fictional_section", true) {
		t.Errorf("owner should bypass section check")
	}
	if !adminSectionAllowed("admin", "fictional_section", true) {
		t.Errorf("admin should bypass section check")
	}
}

// TestRoleAllowed_OwnerImplicit: owner passes any allowed list.
func TestRoleAllowed_OwnerImplicit(t *testing.T) {
	t.Parallel()

	if !roleAllowed("owner", "user") {
		t.Error("owner should be implicitly allowed everywhere")
	}
	if !roleAllowed("owner") {
		t.Error("owner should be allowed even with empty list")
	}
}

// TestRoleAllowed_ExplicitMatch: an explicit role in the list.
func TestRoleAllowed_ExplicitMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		role    string
		allowed []string
		want    bool
	}{
		{"admin", []string{"admin", "support"}, true},
		{"support", []string{"admin", "support"}, true},
		{"tester", []string{"admin"}, false},
		{"user", []string{"admin", "support"}, false},
	}
	for _, c := range cases {
		if got := roleAllowed(c.role, c.allowed...); got != c.want {
			t.Errorf("roleAllowed(%q, %v) = %v, want %v", c.role, c.allowed, got, c.want)
		}
	}
}

// TestValidRole: 8 valid roles + invalid.
func TestValidRole_AllSet(t *testing.T) {
	t.Parallel()

	for _, r := range []string{"user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner"} {
		if !validRole(r) {
			t.Errorf("validRole(%q) = false, want true", r)
		}
	}
	for _, r := range []string{"", "ADMIN", "root", "superuser", "admins"} {
		if validRole(r) {
			t.Errorf("validRole(%q) = true, want false", r)
		}
	}
}

// TestValidStatus: only active/blocked are allowed (pending was removed after migration _06).
func TestValidStatus_OnlyActiveOrBlocked(t *testing.T) {
	t.Parallel()

	if !validStatus("active") || !validStatus("blocked") {
		t.Error("active and blocked must be valid")
	}
	for _, s := range []string{"", "pending", "deleted", "Active", "BLOCKED"} {
		if validStatus(s) {
			t.Errorf("validStatus(%q) = true, want false", s)
		}
	}
}
