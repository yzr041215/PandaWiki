package usecase

import (
	"github.com/google/wire"

	"github.com/chaitin/panda-wiki/repo/ipdb"
	mqRepo "github.com/chaitin/panda-wiki/repo/mq"
	"github.com/chaitin/panda-wiki/repo/pg"
	"github.com/chaitin/panda-wiki/store/rag"
)

var ProviderSet = wire.NewSet(
	pg.ProviderSet,
	mqRepo.ProviderSet,
	ipdb.ProviderSet,
	rag.ProviderSet,

	NewLLMUsecase,
	NewNodeUsecase,
	NewAppUsecase,
	NewConversationUsecase,
	NewUserUsecase,
	NewModelUsecase,
	NewKnowledgeBaseUsecase,
	NewChatUsecase,
	NewCrawlerUsecase,
	NewCreationUsecase,
	NewNotionUsecase,
)
