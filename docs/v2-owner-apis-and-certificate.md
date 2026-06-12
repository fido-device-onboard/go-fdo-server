# V2 Owner APIs 与可选 Owner Certificate 功能文档

## 概述

本功能引入 V2 版本的 Owner API 接口，并支持可选的 Owner Certificate 机制。Owner API 允许资源所有者（Owner）直接管理其拥有的资源，而 Owner Certificate 则提供了一种轻量级的身份验证方式，用于证明 API 调用者的所有者身份。此设计旨在提升 API 的安全性和灵活性，同时降低对中央认证服务的依赖。

### 核心目标
- 提供 V2 版本的 Owner API，增强资源管理能力。
- 引入可选的 Owner Certificate，支持离线或分布式场景下的身份验证。
- 向后兼容：V1 API 继续可用，V2 API 作为增强版本。

## 详细说明

### 1. V2 Owner API 接口

V2 Owner API 是对现有 V1 API 的升级，主要改进包括：
- **资源范围限定**：所有 V2 API 调用必须通过 `owner_id` 参数或路径变量指定资源所有者。
- **增强的响应格式**：返回更详细的资源元数据，如 `created_at`、`updated_at`、`version` 等。
- **批量操作支持**：新增批量创建、更新和删除接口。
- **错误码标准化**：使用统一的错误码格式（如 `OWNER_ERR_xxx`）。

#### 接口列表

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/v2/owners/{owner_id}/resources` | 列出所有资源 |
| POST | `/v2/owners/{owner_id}/resources` | 创建新资源 |
| GET | `/v2/owners/{owner_id}/resources/{resource_id}` | 获取单个资源 |
| PUT | `/v2/owners/{owner_id}/resources/{resource_id}` | 更新资源 |
| DELETE | `/v2/owners/{owner_id}/resources/{resource_id}` | 删除资源 |
| POST | `/v2/owners/{owner_id}/resources/batch` | 批量操作资源 |

### 2. Owner Certificate 机制

Owner Certificate 是一种自签发的数字证书，用于证明调用者拥有某个 `owner_id` 的控制权。证书包含以下字段：

- `owner_id`：所有者标识符
- `public_key`：所有者公钥
- `issued_at`：签发时间
- `expires_at`：过期时间（可选）
- `signature`：使用所有者私钥对上述字段的签名

证书可以存储在本地文件或环境变量中，API 调用时通过 `X-Owner-Certificate` 请求头传递。

#### 验证流程

1. 客户端生成 RSA 密钥对（2048 位或更高）。
2. 客户端创建证书 JSON 对象，并使用私钥签名。
3. 客户端在 API 请求中附带证书和签名。
4. 服务端验证证书签名、检查 `owner_id` 是否匹配，并确认证书未过期。

### 3. 配置与集成

#### 服务端配置

在服务端配置文件中启用 V2 API 和证书验证：

```yaml
# config.yaml
api:
  version: v2
  owner_certificate:
    enabled: true
    allowed_issuers: ["self"]
    max_cert_age_hours: 24
```

#### 客户端配置

客户端需要生成证书并配置到 API 调用中：

```bash
# 生成密钥对
openssl genrsa -out owner_private.pem 2048
openssl rsa -in owner_private.pem -pubout -out owner_public.pem

# 创建证书（示例脚本）
python create_cert.py --owner-id user123 --private-key owner_private.pem --output cert.json
```

## 示例

### 示例 1：使用 Owner Certificate 调用 V2 API

```python
import requests
import json

# 加载证书
with open('cert.json', 'r') as f:
    cert = json.load(f)

# 准备请求
owner_id = cert['owner_id']
url = f"https://api.example.com/v2/owners/{owner_id}/resources"
headers = {
    "X-Owner-Certificate": json.dumps(cert),
    "Content-Type": "application/json"
}

# 创建资源
payload = {
    "name": "my-resource",
    "type": "document",
    "metadata": {
        "description": "Test resource"
    }
}

response = requests.post(url, headers=headers, json=payload)
print(response.json())
```

### 示例 2：批量操作资源

```bash
curl -X POST "https://api.example.com/v2/owners/user123/resources/batch" \
  -H "X-Owner-Certificate: $(cat cert.json | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "operations": [
      {"action": "create", "data": {"name": "res1", "type": "file"}},
      {"action": "create", "data": {"name": "res2", "type": "image"}},
      {"action": "delete", "resource_id": "old-res-001"}
    ]
  }'
```

### 示例 3：证书生成脚本

```python
# create_cert.py
import argparse
import json
import time
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa, padding
from cryptography.hazmat.backends import default_backend

def create_cert(owner_id, private_key_path, output_path, expires_in_hours=24):
    # 加载私钥
    with open(private_key_path, 'rb') as f:
        private_key = serialization.load_pem_private_key(
            f.read(),
            password=None,
            backend=default_backend()
        )
    
    # 获取公钥
    public_key = private_key.public_key()
    public_key_pem = public_key.public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo
    ).decode()
    
    # 构建证书
    cert = {
        "owner_id": owner_id,
        "public_key": public_key_pem,
        "issued_at": int(time.time()),
        "expires_at": int(time.time()) + expires_in_hours * 3600
    }
    
    # 签名
    cert_json = json.dumps(cert, sort_keys=True).encode()
    signature = private_key.sign(
        cert_json,
        padding.PSS(
            mgf=padding.MGF1(hashes.SHA256()),
            salt_length=padding.PSS.MAX_LENGTH
        ),
        hashes.SHA256()
    )
    cert["signature"] = signature.hex()
    
    # 输出
    with open(output_path, 'w') as f:
        json.dump(cert, f, indent=2)
    
    print(f"Certificate created for owner {owner_id} at {output_path}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--owner-id", required=True)
    parser.add_argument("--private-key", required=True)
    parser.add_argument("--output", default="cert.json")
    parser.add_argument("--expires", type=int, default=24)
    args = parser.parse_args()
    
    create_cert(args.owner_id, args.private_key, args.output, args.expires)
```

## 注意事项

### 1. 安全考量
- **私钥保护**：Owner Certificate 的私钥必须严格保密，建议使用硬件安全模块（HSM）或密钥管理服务（KMS）。
- **证书过期**：设置合理的过期时间（建议 24 小时以内），并实现证书轮换机制。
- **防重放攻击**：在证书中包含时间戳，服务端应拒绝过期的证书。
- **HTTPS 强制**：所有 V2 API 调用必须通过 HTTPS，防止证书在传输中被截获。

### 2. 性能影响
- 证书验证会增加请求延迟（约 5-10ms），建议在服务端实现缓存机制。
- 批量操作接口可能产生大量数据库写入，建议限制单次批量操作的数量（如最多 100 条）。

### 3. 兼容性
- V1 API 将继续支持，但不会获得 V2 的新功能。
- 迁移到 V2 时，建议先并行运行两个版本，逐步将客户端迁移至 V2。
- 如果不使用 Owner Certificate，V2 API 仍支持传统的 API Key 认证方式。

### 4. 错误处理
- 证书无效时，返回 HTTP 401 状态码和错误码 `OWNER_ERR_INVALID_CERT`。
- 证书过期时，返回 HTTP 401 和错误码 `OWNER_ERR_CERT_EXPIRED`。
- 资源不存在时，返回 HTTP 404 和错误码 `OWNER_ERR_RESOURCE_NOT_FOUND`。

### 5. 日志与监控
- 记录所有 V2 API 调用日志，包括 `owner_id`、`resource_id`、操作类型和结果。
- 监控证书验证失败率，异常升高可能表示安全攻击。
- 设置告警规则：连续 5 次证书验证失败触发告警。

### 6. 测试建议
- 使用测试证书（过期时间为 1 分钟）验证过期逻辑。
- 模拟密钥泄露场景，测试证书撤销机制（如果实现）。
- 使用负载测试工具验证批量接口的性能瓶颈。