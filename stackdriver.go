package logrus

import (
	"io/ioutil"
	"os"
	"regexp"
	"sync"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/logging"
	"golang.org/x/net/context"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

var initHook sync.Once

func init() {
	initHook.Do(testAndSetHook)
}

func testAndSetHook() {
	if metadata.OnGCE() {
		projectID, err := metadata.ProjectID()
		if err != nil {
			Panic(err)
		}
		instance, err := metadata.InstanceName()
		if err != nil {
			Panic(err)
		}
		namespace := os.Getenv("POD_NAMESPACE")
		if namespace == "" {
			Panic("POD_NAMESPACE not set. Please define it in the yaml file using downward API")
		}
		pod := os.Getenv("POD_NAME")
		if pod == "" {
			Panic("POD_NAME not set. Please define it in the yaml file using downward API")
		}
		AddHook(newHook(projectID, toClusterId(instance), namespace, toComponentName(pod)))
		SetOutput(ioutil.Discard)
	}
}

func toClusterId(instanceName string) string {
	re := regexp.MustCompile("^gke-(.+)-.+-pool-.+")
	match := re.FindStringSubmatch(instanceName)
	if len(match) == 1 || len(match) > 2 {
		Panic("Convert instance to cluster id failed")
	}
	return match[1]
}

func toComponentName(podName string) string {
	re := regexp.MustCompile("^(.+)-.+-.+")
	match := re.FindStringSubmatch(podName)
	if len(match) == 1 || len(match) > 2 {
		Panic("Convert pod name to component name failed")
	}
	return match[1]
}

type stackDriverHook struct {
	levels    []Level
	projectID string
	cluster   string
	namespace string
	component string
	logger    *logging.Logger
}

func (sh *stackDriverHook) Levels() []Level {
	return sh.levels
}

func (sh *stackDriverHook) Fire(entry *Entry) error {
	sh.logger.Log(logging.Entry{Payload: entry.Message, Severity: toSeverity(entry.Level)})
	return nil
}

func newHook(projectID, cluster, namespace, component string) *stackDriverHook {
	sh := &stackDriverHook{
		levels:    AllLevels,
		projectID: projectID,
		cluster:   cluster,
		namespace: namespace,
		component: component,
	}
	client, err := logging.NewClient(context.Background(), projectID)
	if err != nil {
		Panic("new client create error: %v", err)
	}
	res := &mrpb.MonitoredResource{
		Type: "container",
		Labels: map[string]string{
			"cluster_name": cluster,
			"namespace_id": namespace,
		},
	}
	sh.logger = client.Logger(component, logging.CommonResource(res))
	return sh
}

func toSeverity(level Level) logging.Severity {
	switch level {
	case PanicLevel:
		return logging.Critical
	case FatalLevel:
		return logging.Critical
	case ErrorLevel:
		return logging.Error
	case WarnLevel:
		return logging.Warning
	case InfoLevel:
		return logging.Info
	case DebugLevel:
		return logging.Debug
	default:
		return logging.Default
	}
}
