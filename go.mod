module github.com/matsest/lazyazure

go 1.26.1

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.21.1
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions v1.3.0
	github.com/alecthomas/chroma/v2 v2.23.1
	github.com/atotto/clipboard v0.1.4
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/jesseduffield/gocui v0.3.1-0.20260327132312-944dab3bc980
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.7.1 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/gdamore/tcell/v2 v2.13.8 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/term v0.42.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace github.com/jesseduffield/gocui => ./vendor_gocui
