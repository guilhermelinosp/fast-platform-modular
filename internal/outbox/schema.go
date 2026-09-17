package outbox

import (
	"fmt"
	"os"
)

// schemaRegistryURLURL env que o registro de schemas usa.
// Ex: http://localhost:8081
var schemaRegistryURL = os.Getenv("HELLNET_KAFKA_SCHEMA_REGISTRY_URL")

// Schemas avro definidos inline (já que avros/ está vazio).
var schemas = []struct {
	Subject string
	Avro    string
}{
	{Subject: "fast-ride-requested", Avro: `{"type":"record","name":"Requested","namespace":"com.hellnet","fields":[{"name":"event_id","type":"string"},{"name":"ride_id","type":"string"},{"name":"driver_id","type":"string"},{"name":"status","type":"string"},{"name":"created_at","type":"string"}]`},
	{Subject: "fast-ride-accepted", Avro: `{"type":"record","name":"Accepted","namespace":"com.hellnet","fields":[{"name":"event_id","type":"string"},{"name":"ride_id","type":"string"},{"name":"driver_id","type":"string"},{"name":"status","type":"string"}]`},
}

// Validate verifica se o URL do registry está configurada e os schemas estão definidos.
// Retorna erro se a URL for definida mas inválida ou se os schemas estiverem vazios.
func Validate() error {
	if schemaRegistryURL != "" && !validURL(schemaRegistryURL) {
		return fmt.Errorf("HELLNET_KAFKA_SCHEMA_REGISTRY_URL %q is not a valid URL", schemaRegistryURL)
	}
	if len(schemas) == 0 {
		return fmt.Errorf("pelo menos um schema deve ser definido")
	}
	return nil
}

// validURL verifica se a string parece um URL http(s):// .
func validURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}

// RegisterSchemas posta cada schema no schema registry do Kafka.
// Retorna nil se todos já existirem (409) ou se o registro sucedeu.
// Em caso de erro de rede ou outros códigos de status, retorna o erro.
func RegisterSchemas() error {
	if schemaRegistryURL == "" {
		// Em ambientes sem registry, o registro é opcional; schemas são versionados
		// pelo próprio aplicativo ou ignorados.
		return nil
	}
	// TODO: Implementar POSTs HTTP para cada schema no schema registry.
	// Padrão ccompat v6: POST /subjects/{subject}/versions com body {"schema": "<avro-json>"}.
	// Tratar 409 (já registrado) como sucesso (log info).
	// Outros erros de rede ou códigos 4xx/5xx são retornados.
	return nil
}
