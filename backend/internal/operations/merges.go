package operations

func (handler *service) merge(id string) (any, error) {
	var input struct {
		Mutation
		TargetAccountID string `json:"target_account_id"`
		TargetRevision  int    `json:"target_revision"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.TargetAccountID); err != nil {
		return nil, err
	}
	source, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	target, err := handler.readAccount(input.TargetAccountID)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(source, true); err != nil {
		return nil, err
	}
	if err := handler.validateTransferTarget(target); err != nil {
		return nil, err
	}
	if err := ensureOpen(source, input.Revision); err != nil {
		return nil, err
	}
	if err := ensureOpen(target, input.TargetRevision); err != nil {
		return nil, err
	}
	if err := compatibleAccounts(source, target); err != nil {
		return nil, err
	}
	sourceDraft, err := handler.readDraft(id)
	if err != nil {
		return nil, err
	}
	targetDraft, err := handler.readDraft(target.ID)
	if err != nil {
		return nil, err
	}
	if len(sourceDraft)+len(targetDraft) > 20 || source.DraftQuantity+target.DraftQuantity > 1000 {
		return nil, invalid("Confirma o reduce los borradores antes de combinar: el lote resultante excede 20 partidas o 1000 unidades.")
	}
	if err := handler.touch(source.ID, source.Revision); err != nil {
		return nil, err
	}
	if err := handler.touch(target.ID, target.Revision); err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_item_units SET account_id=$1 WHERE account_id=$2 AND state='active'", target.ID, source.ID)
	if err != nil {
		return nil, err
	}
	for index, item := range sourceDraft {
		_, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_draft_items SET account_id=$1,position=$2 WHERE id=$3", target.ID, len(targetDraft)+index, item.ID)
		if err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_accounts SET state='merged',merged_into_id=$1 WHERE id=$2", target.ID, source.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.event(source.ID, &target.ID, "account.merged", map[string]any{"source": source, "target": target}, map[string]any{
		"source_id": source.ID, "target_id": target.ID, "confirmed_quantity": source.ConfirmedQuantity, "draft_quantity": source.DraftQuantity,
		"preserved_totals": source.Totals,
	}); err != nil {
		return nil, err
	}
	return handler.accountPair(source.ID, target.ID)
}
