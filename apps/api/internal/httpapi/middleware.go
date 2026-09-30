package httpapi

import "net/http"

func withCORS(next http.Handler, allowedOrigins map[string]struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowedOrigins[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			} else {
				// K-4: Origin is present but not in the allowlist: block mutating
				// requests from foreign origins. Requests without Origin (curl, server-side
				// clients, YooKassa) pass straight through.
				switch r.Method {
				case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
