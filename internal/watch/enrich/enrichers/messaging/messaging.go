package messaging

import (
	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher {
	return append(pattern.FromSpecs(Specs()), ConcreteBindings())
}

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.kafkajs", "TypeScript KafkaJS", "typescript", "kafkajs", "Kafka(", "messaging.topic", "publishes"),
		spec("ts.bullmq_messaging", "TypeScript BullMQ messaging", "typescript", "bullmq", "new Queue", "messaging.queue", "publishes"),
		spec("ts.aws_sqs", "TypeScript AWS SQS SDK", "typescript", "@aws-sdk/client-sqs", "SQSClient", "messaging.queue", "publishes"),
		spec("ts.amqplib", "TypeScript amqplib", "typescript", "amqplib", "amqplib", "messaging.queue", "publishes"),
		spec("ts.nats", "TypeScript NATS", "typescript", "nats", "connect(", "messaging.topic", "subscribes_to"),
		spec("go.kafka_go", "Go kafka-go", "go", "github.com/segmentio/kafka-go", "kafka.Writer", "messaging.topic", "publishes"),
		spec("go.sarama", "Go Sarama", "go", "github.com/IBM/sarama", "sarama.New", "messaging.topic", "publishes"),
		spec("go.nats", "Go NATS", "go", "github.com/nats-io/nats.go", "nats.Connect", "messaging.topic", "subscribes_to"),
		spec("go.rabbitmq", "Go RabbitMQ", "go", "github.com/rabbitmq/amqp091-go", "amqp.Dial", "messaging.queue", "consumes"),
		spec("go.aws_sqs", "Go AWS SQS SDK", "go", "github.com/aws/aws-sdk-go-v2/service/sqs", "sqs.NewFromConfig", "messaging.queue", "publishes"),
		spec("python.celery_messaging", "Python Celery messaging", "python", "celery", "Celery(", "messaging.queue", "consumes"),
		spec("python.kafka_python", "Python kafka-python", "python", "kafka-python", "KafkaProducer", "messaging.topic", "publishes"),
		spec("python.confluent_kafka", "Python confluent-kafka", "python", "confluent-kafka", "confluent_kafka", "messaging.topic", "publishes"),
		spec("python.pika", "Python pika", "python", "pika", "pika.", "messaging.queue", "consumes"),
		spec("python.boto3_sqs", "Python boto3 SQS", "python", "boto3", "sqs", "messaging.queue", "publishes"),
		spec("java.spring_kafka", "Java Spring Kafka", "java", "spring-kafka", "KafkaTemplate", "messaging.topic", "publishes"),
		spec("java.kafka_clients", "Java Kafka clients", "java", "org.apache.kafka", "KafkaProducer", "messaging.topic", "publishes"),
		spec("java.spring_amqp", "Java Spring AMQP", "java", "spring-amqp", "RabbitTemplate", "messaging.queue", "publishes"),
		spec("java.jms", "Java JMS", "java", "jakarta.jms", "JMSContext", "messaging.queue", "publishes"),
		spec("java.aws_sqs", "Java AWS SQS SDK", "java", "software.amazon.awssdk.services.sqs", "SqsClient", "messaging.queue", "publishes"),
		spec("rust.rdkafka", "Rust rdkafka", "rust", "rdkafka", "rdkafka::", "messaging.topic", "publishes"),
		spec("rust.lapin", "Rust lapin", "rust", "lapin", "lapin::", "messaging.queue", "consumes"),
		spec("rust.async_nats", "Rust async-nats", "rust", "async-nats", "async_nats::", "messaging.topic", "subscribes_to"),
		spec("rust.aws_sqs", "Rust AWS SQS SDK", "rust", "aws-sdk-sqs", "aws_sdk_sqs::", "messaging.queue", "publishes"),
		spec("cpp.librdkafka", "C++ librdkafka", "cpp", "librdkafka", "RdKafka::", "messaging.topic", "publishes"),
		spec("cpp.rabbitmq_c", "C++ rabbitmq-c", "cpp", "rabbitmq-c", "amqp_login", "messaging.queue", "consumes"),
		spec("cpp.nats", "C++ nats.cpp", "cpp", "nats.cpp", "natsConnection", "messaging.topic", "subscribes_to"),
		spec("cpp.aws_sqs", "C++ AWS SQS SDK", "cpp", "aws-sdk-cpp", "Aws::SQS", "messaging.queue", "publishes"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "messaging",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"messaging:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
