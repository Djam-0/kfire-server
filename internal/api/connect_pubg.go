package api

import (
	"errors"
	"log/slog"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/knightsofeternity/kfire-server/internal/connectors/pubg"
	"github.com/knightsofeternity/kfire-server/internal/store"
)

// maxPubgNameBytes bounds the typed in-game name before it reaches PUBG. The
// publisher caps names far below this; the bound exists so a pasted novel is
// rejected here rather than spent against a ten-a-minute quota.
const maxPubgNameBytes = 64

// validPubgPlatform reports whether p is a shard KFIRE routes to. The value
// reaches a URL path, so it is checked against the known list rather than
// escaped.
func validPubgPlatform(p string) bool {
	return slices.Contains(pubg.KnownPlatforms(), p)
}

// pubgNotFound answers the single most common failure: the member mistyped
// their name, or picked the wrong platform.
//
// It is deliberately a different status and a different code from a connector
// failure. Told "something went wrong", a member waits for a fix that is never
// coming, because the only thing that can fix it is them.
func pubgNotFound(c *fiber.Ctx) error {
	return errorJSON(c, fiber.StatusNotFound, "pubg_name_not_found",
		"PUBG does not know this name on this platform")
}

// pubgUnavailable answers a failure on PUBG's side: a timeout, a 5xx, a
// saturated quota. Nothing the member typed is wrong, and trying again later
// is the right move.
func pubgUnavailable(c *fiber.Ctx, userID string, err error) error {
	slog.Warn("pubg: resolve player", "user_id", userID, "err", err)
	return errorJSON(c, fiber.StatusBadGateway, "pubg_unavailable",
		"PUBG did not answer, try again in a moment")
}

// POST /api/v1/connect/pubg  (authenticated)
//
// Links the caller's KFIRE account to a PUBG account resolved from a typed
// in-game name and a platform. PUBG's player search is public and needs no
// consent from the player, so linking trusts the name the member types, the
// same way Riot's does here.
//
// What lands in linked_accounts is the ACCOUNT ID, never the name. A member
// who renames themself in game keeps their link and their history; storing the
// name would break both on the first rename.
func (h *handlers) connectPubg(c *fiber.Ctx) error {
	if h.pubg == nil || !h.pubg.Enabled() {
		return errorJSON(c, fiber.StatusNotImplemented, "connector_disabled",
			"the PUBG connector is not configured on this instance")
	}

	var body struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}
	if err := c.BodyParser(&body); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, "invalid_pubg_name",
			"name must be your in-game PUBG name")
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > maxPubgNameBytes {
		return errorJSON(c, fiber.StatusBadRequest, "invalid_pubg_name",
			"name must be your in-game PUBG name")
	}
	if !validPubgPlatform(body.Platform) {
		return errorJSON(c, fiber.StatusBadRequest, "invalid_platform",
			"platform must be one of the supported PUBG shards")
	}

	userID := mustClaims(c).UserID
	player, err := h.pubg.PlayerByName(body.Platform, name)
	if pubg.NotFound(err) {
		return pubgNotFound(c)
	}
	if err != nil {
		return pubgUnavailable(c, userID, err)
	}

	// One PUBG account per member.
	taken, err := h.store.ProviderLinkedToOther(c.Context(), "pubg", player.AccountID, userID)
	if err != nil {
		return err
	}
	if taken {
		return errorJSON(c, fiber.StatusConflict, "already_linked",
			"this PUBG account is already linked to another member")
	}

	account := store.LinkedAccount{
		Provider:       "pubg",
		ProviderUserID: player.AccountID,
		// The name PUBG answers with, not the one that was typed: it carries
		// the real casing, and it is only ever shown back to the member.
		DisplayName: strPtr(player.Name),
	}
	if err := h.store.UpsertLinkedAccount(c.Context(), userID, account); err != nil {
		return err
	}
	if err := h.store.UpsertPubgAccount(c.Context(), userID, body.Platform); err != nil {
		// PubgAccountFor joins identity and routing, so an identity row with no
		// routing row reads as "not linked": the member would see a link they
		// cannot use and the sync would never pick them up. Roll it back.
		if derr := h.store.DeleteLinkedAccount(c.Context(), userID, "pubg"); derr != nil {
			slog.Error("pubg: roll back orphaned link", "user_id", userID, "err", derr)
		}
		return err
	}
	return c.JSON(fiber.Map{"name": player.Name, "platform": body.Platform})
}

// DELETE /api/v1/connect/pubg  (authenticated)
//
// Stops the collection and drops the link. The matches already recorded are
// KEPT, on purpose: PUBG deletes a match after 14 days, publisher included, so
// deleting here would destroy history that nothing could ever fetch again,
// including a member who relinks the next day. That is not something anyone
// can anticipate from an "unlink" button.
func (h *handlers) disconnectPubg(c *fiber.Ctx) error {
	userID := mustClaims(c).UserID
	// Routing goes first. Deleting it is a no-op when there is nothing to
	// delete, so a failure here leaves the identity row in place and the member
	// can simply retry. The reverse order would strand the routing row: with
	// the identity already gone, a second attempt answers "not linked".
	if err := h.store.DeletePubgAccount(c.Context(), userID); err != nil {
		return err
	}
	err := h.store.DeleteLinkedAccount(c.Context(), userID, "pubg")
	if errors.Is(err, store.ErrNotFound) {
		return errorJSON(c, fiber.StatusNotFound, "not_linked", "no PUBG account linked")
	}
	if err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GET /api/v1/connect/pubg/platform  (authenticated)
//
// Returns the shard the member plays on, so the account page's picker opens on
// their platform rather than on a default.
//
// Like Riot's region, the platform deliberately does not ride along in
// connectionJSON: that shape is shared with the public API, and routing is
// internal plumbing no third party needs.
func (h *handlers) pubgPlatform(c *fiber.Ctx) error {
	platform, _, err := h.store.PubgAccountFor(c.Context(), mustClaims(c).UserID)
	if err != nil {
		return errorJSON(c, fiber.StatusNotFound, "not_linked", "no PUBG account linked")
	}
	return c.JSON(fiber.Map{"platform": platform})
}

// PATCH /api/v1/connect/pubg/platform  (authenticated)
//
// Corrects the shard when the member picked the wrong one.
//
// Unlike Riot's region, this cannot just rewrite the routing row: a PUBG
// account id is issued by a shard and means nothing on another one, so keeping
// the old id under a new platform would leave a link that resolves to nobody.
// The name is therefore resolved again on the new shard, and the id is
// replaced with what comes back. Nothing is written when it resolves to
// nobody, so a wrong correction leaves a working link alone.
func (h *handlers) updatePubgPlatform(c *fiber.Ctx) error {
	if h.pubg == nil || !h.pubg.Enabled() {
		return errorJSON(c, fiber.StatusNotImplemented, "connector_disabled",
			"the PUBG connector is not configured on this instance")
	}
	var body struct {
		Platform string `json:"platform"`
	}
	if err := c.BodyParser(&body); err != nil || !validPubgPlatform(body.Platform) {
		return errorJSON(c, fiber.StatusBadRequest, "invalid_platform",
			"platform must be one of the supported PUBG shards")
	}

	userID := mustClaims(c).UserID
	linked, err := h.store.GetLinkedAccount(c.Context(), userID, "pubg")
	if err != nil || linked.DisplayName == nil {
		// No stored name to look up again: the member unlinks and links again,
		// which is the path that asks for the name in the first place.
		return errorJSON(c, fiber.StatusNotFound, "not_linked", "no PUBG account linked")
	}

	player, err := h.pubg.PlayerByName(body.Platform, *linked.DisplayName)
	if pubg.NotFound(err) {
		return pubgNotFound(c)
	}
	if err != nil {
		return pubgUnavailable(c, userID, err)
	}
	taken, err := h.store.ProviderLinkedToOther(c.Context(), "pubg", player.AccountID, userID)
	if err != nil {
		return err
	}
	if taken {
		return errorJSON(c, fiber.StatusConflict, "already_linked",
			"this PUBG account is already linked to another member")
	}

	if err := h.store.UpsertLinkedAccount(c.Context(), userID, store.LinkedAccount{
		Provider:       "pubg",
		ProviderUserID: player.AccountID,
		DisplayName:    strPtr(player.Name),
	}); err != nil {
		return err
	}
	if err := h.store.UpsertPubgAccount(c.Context(), userID, body.Platform); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"name": player.Name, "platform": body.Platform})
}
