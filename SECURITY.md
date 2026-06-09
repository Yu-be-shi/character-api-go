# セキュリティポリシー

## 脆弱性の報告
脆弱性を見つけた場合は、公開 Issue ではなく非公開（リポジトリオーナーへの連絡 / GitHub Security Advisory）で報告してください。

## 自動スキャン
- 依存の脆弱性：dependabot / `govulncheck`（Go）/ `npm audit`（Node）
- SAST：CodeQL / gosec（Go）
- 秘密情報：gitleaks
- IaC：trivy（Terraform。infra リポジトリ）

## 原則
- シークレットはコミットしない（`.env` 等は gitignore 済み）。`INTERNAL_API_KEY` 等は Secrets Manager / GitHub Secrets で管理。
- `/api/v1/*` は `X-Internal-API-Key` 必須。認可は UI だけでなくサーバー側でも検証する。
