// Package port reúne TODAS as portas da aplicação, um arquivo por assunto: repositórios por agregado e
// transação (repository.go), KMS (kms.go), emissão e validação de tokens (token.go), métricas (metrics.go),
// relógio (clock.go), erros compartilhados (errors.go) e a fase da chamada ao KMS carregada no contexto
// (phase.go). Os casos de uso e o transporte dependem só deste pacote (e do domínio); os adaptadores em
// internal/adapter os implementam.
package port
