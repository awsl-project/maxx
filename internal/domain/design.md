1. 首先是一个高性能的 http proxy server
2. 基于不同的特征判断是哪种 ClinetType
3. 基于 Route 表，依次匹配并请求
4. 基于不同的 ProviderType，实现不同的 Adaptor，应对不同的 ClientType 请求
