function buildHeaders(extra = {}) {
  const headers = { ...extra };
  const token = localStorage.getItem("authToken");
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

function normalizeApiUrl(url) {
  if (typeof url !== "string") return url;
  if (url.startsWith("/api/v1/")) return url;
  if (url === "/api") return "/api/v1";
  if (url.startsWith("/api/")) return url.replace("/api/", "/api/v1/");
  return url;
}

export async function getJSON(url) {
  const res = await fetch(normalizeApiUrl(url), {
    headers: buildHeaders(),
  });
  return { ok: res.ok, status: res.status, data: await res.json() };
}

export async function postJSON(url, payload) {
  const res = await fetch(normalizeApiUrl(url), {
    method: "POST",
    headers: buildHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(payload),
  });
  return { ok: res.ok, status: res.status, data: await res.json() };
}
