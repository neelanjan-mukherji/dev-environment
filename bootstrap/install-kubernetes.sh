#!/usr/bin/env bash
set -Eeuo pipefail
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.34/deb/Release.key | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
sudo chmod 644 /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.34/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list >/dev/null
curl -fsSL https://baltocdn.com/helm/signing.asc | sudo gpg --dearmor --yes -o /usr/share/keyrings/helm.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list >/dev/null
sudo apt-get update && sudo apt-get install -y kubectl helm
if ! command -v k9s >/dev/null; then
 arch="$(dpkg --print-architecture)"; case "$arch" in amd64) a=amd64;; arm64) a=arm64;; *) echo "Skipping k9s for $arch"; exit 0;; esac
 tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
 url="$(curl -fsSL https://api.github.com/repos/derailed/k9s/releases/latest | jq -r '.assets[] | select(.name | test("Linux_'"$a"'.tar.gz$")) | .browser_download_url' | head -n1)"
 [[ -n "$url" && "$url" != null ]] && { curl -fsSL "$url" -o "$tmp/k9s.tgz"; tar -xzf "$tmp/k9s.tgz" -C "$tmp" k9s; sudo install -m 0755 "$tmp/k9s" /usr/local/bin/k9s; }
fi
