module github.com/flux-iac/tofu-controller

go 1.25.6

replace github.com/flux-iac/tofu-controller/api => ./api

replace github.com/flux-iac/tofu-controller/tfctl => ./tfctl

require (
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/aws/aws-sdk-go-v2 v1.41.1
	github.com/aws/aws-sdk-go-v2/config v1.32.7
	github.com/aws/aws-sdk-go-v2/credentials v1.19.7
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.54.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.96.0
	github.com/aws/smithy-go v1.24.0
	github.com/cyphar/filepath-securejoin v0.6.1
	github.com/elgohr/go-localstack v1.0.147
	github.com/flux-iac/tofu-controller/api v0.0.0-20250321074822-67198a27f35b
	github.com/flux-iac/tofu-controller/tfctl v0.0.0-00010101000000-000000000000
	github.com/fluxcd/pkg/apis/meta v1.25.0
	github.com/fluxcd/pkg/runtime v0.97.0
	github.com/fluxcd/pkg/tar v0.17.0
	github.com/fluxcd/source-controller/api v1.7.4
	github.com/go-logr/logr v1.4.3
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/go-retryablehttp v0.7.8
	github.com/hashicorp/hcl2 v0.0.0-20191002203319-fb75b3253c80
	github.com/hashicorp/terraform-exec v0.24.0
	github.com/hashicorp/terraform-json v0.27.2
	github.com/jenkins-x/go-scm v1.15.16
	github.com/kubescape/go-git-url v0.0.31
	github.com/maxbrunsfeld/counterfeiter/v6 v6.12.1
	github.com/onsi/gomega v1.39.0
	github.com/spf13/afero v1.15.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/zclconf/go-cty v1.17.0
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.35.0
	k8s.io/apiextensions-apiserver v0.35.0
	k8s.io/apimachinery v0.35.0
	k8s.io/cli-runtime v0.35.0
	k8s.io/client-go v0.35.0
	k8s.io/kubectl v0.35.0
	k8s.io/utils v0.0.0-20260106112306-0fe9cd71b2f8
	sigs.k8s.io/cli-utils v0.37.2
	sigs.k8s.io/controller-runtime v0.23.0
	sigs.k8s.io/kustomize/kyaml v0.21.0
)

require (
	code.gitea.io/sdk/gitea v0.20.0 // indirect
	dario.cat/mergo v1.0.2 // indirect
	fortio.org/safecast v1.2.0 // indirect
	github.com/42wim/httpsig v1.2.3 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/agext/levenshtein v1.2.1 // indirect
	github.com/apparentlymart/go-textseg v1.0.0 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.4 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.53.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.40.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudwatchevents v1.25.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.39.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.22.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.177.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticsearchservice v1.30.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/firehose v1.32.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/iam v1.35.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/kinesis v1.29.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/lambda v1.58.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/redshift v1.46.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/route53 v1.43.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.32.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ses v1.26.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sfn v1.31.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.31.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.34.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.52.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.6 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bluekeyes/go-gitdiff v0.8.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/gettext-go v1.0.3 // indirect
	github.com/chainguard-dev/git-urls v1.0.2 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/davidmz/go-pageant v1.0.2 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v28.5.2+incompatible // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fluxcd/cli-utils v0.37.1-flux.1 // indirect
	github.com/fluxcd/pkg/apis/acl v0.9.0 // indirect
	github.com/fluxcd/pkg/apis/event v0.22.0 // indirect
	github.com/fluxcd/pkg/ssa v0.65.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-fed/httpsig v1.1.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.4 // indirect
	github.com/go-openapi/jsonreference v0.21.4 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hako/durafmt v0.0.0-20210608085754-5c1018a4e16b // indirect
	github.com/hashicorp/go-version v1.8.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/olekukonko/tablewriter v1.0.8 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/shurcooL/githubv4 v0.0.0-20240727222349-48295856cce7 // indirect
	github.com/shurcooL/graphql v0.0.0-20230722043721-ed46e5a46466 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/theckman/yacspin v0.13.12 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.starlark.net v0.0.0-20260102030733-3fee463870c9 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/term v0.39.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251222181119-0a764e51fe1b // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/evanphx/json-patch.v5 v5.9.11 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/component-base v0.35.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20251125145642-4e65d59e963e // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/kustomize/api v0.21.0 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.1 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

// https://github.com/darccio/mergo?tab=readme-ov-file#100
replace github.com/imdario/mergo => github.com/imdario/mergo v0.3.16

// Pin kustomize to v5.3.0
replace (
	sigs.k8s.io/kustomize/api => sigs.k8s.io/kustomize/api v0.16.0
	sigs.k8s.io/kustomize/kyaml => sigs.k8s.io/kustomize/kyaml v0.16.0
)

// indirect - breaking changes in v1.0.x
replace github.com/olekukonko/tablewriter => github.com/olekukonko/tablewriter v0.0.5

// https://github.com/maxbrunsfeld/counterfeiter/issues/344
replace golang.org/x/tools => golang.org/x/tools v0.37.0
