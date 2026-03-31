package mysqlstore

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"quant-system/pkg/contracts"
)

func TestRepositoryRecoveryFromMySQL(t *testing.T) {
	if os.Getenv("RUN_MYSQL_TESTS") != "1" {
		t.Skip("skip mysql integration test (set RUN_MYSQL_TESTS=1)")
	}

	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3306)/quant?parseTime=true&multiStatements=true"
	}

	db, err := Open(Config{
		DSN:             dsn,
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	repo, err := NewRepository(db)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	ctx := context.Background()
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	if err := truncateAll(ctx, db); err != nil {
		t.Fatalf("truncateAll() error = %v", err)
	}

	order := contracts.Order{
		ClientOrderID: "cid-r-1",
		VenueOrderID:  "vo-r-1",
		Symbol:        "BTC-USDT",
		State:         contracts.OrderStateFilled,
		FilledQty:     0.2,
		AvgPrice:      62000,
		StateVersion:  3,
		UpdatedMS:     time.Now().UnixMilli(),
	}
	if err := repo.UpsertOrder(ctx, order); err != nil {
		t.Fatalf("UpsertOrder() error = %v", err)
	}

	pos := contracts.PositionSnapshot{
		AccountID:   "acc-r-1",
		Symbol:      "BTC-USDT",
		Quantity:    0.2,
		AvgCost:     62000,
		RealizedPnL: 0,
		UpdatedMS:   time.Now().UnixMilli(),
	}
	if err := repo.UpsertPosition(ctx, pos); err != nil {
		t.Fatalf("UpsertPosition() error = %v", err)
	}

	decision := contracts.RiskDecision{
		Intent: contracts.OrderIntent{
			IntentID:   "intent-r-1",
			StrategyID: "s-r-1",
			Symbol:     "BTC-USDT",
			Side:       "buy",
			Price:      62000,
			Quantity:   0.2,
		},
		Decision:    contracts.RiskDecisionAllow,
		RuleID:      "risk.pass",
		ReasonCode:  "ok",
		EvaluatedMS: time.Now().UnixMilli(),
	}
	if err := repo.SaveRiskDecision(ctx, decision); err != nil {
		t.Fatalf("SaveRiskDecision() error = %v", err)
	}

	// Simulate restart by creating a new repository from same DB.
	reloadedRepo, err := NewRepository(db)
	if err != nil {
		t.Fatalf("NewRepository(reloaded) error = %v", err)
	}

	gotOrder, ok, err := reloadedRepo.GetOrder(ctx, "cid-r-1")
	if err != nil || !ok {
		t.Fatalf("GetOrder() err=%v ok=%v", err, ok)
	}
	if gotOrder.State != contracts.OrderStateFilled || gotOrder.FilledQty != 0.2 {
		t.Fatalf("unexpected order snapshot: %+v", gotOrder)
	}

	gotPos, ok, err := reloadedRepo.GetPosition(ctx, "acc-r-1", "BTC-USDT")
	if err != nil || !ok {
		t.Fatalf("GetPosition() err=%v ok=%v", err, ok)
	}
	if gotPos.Quantity != 0.2 || gotPos.AvgCost != 62000 {
		t.Fatalf("unexpected position snapshot: %+v", gotPos)
	}

	gotDecision, ok, err := reloadedRepo.GetRiskDecision(ctx, "intent-r-1")
	if err != nil || !ok {
		t.Fatalf("GetRiskDecision() err=%v ok=%v", err, ok)
	}
	if gotDecision.Decision != contracts.RiskDecisionAllow || gotDecision.RuleID != "risk.pass" {
		t.Fatalf("unexpected risk decision snapshot: %+v", gotDecision)
	}
}

func truncateAll(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		"TRUNCATE TABLE risk_decisions",
		"TRUNCATE TABLE positions",
		"TRUNCATE TABLE orders",
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
