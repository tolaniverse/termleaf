package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tolaniverse/termleaf/internal/app"
	"github.com/tolaniverse/termleaf/internal/document"
	"github.com/tolaniverse/termleaf/internal/page"
	"github.com/tolaniverse/termleaf/internal/render"
	"github.com/tolaniverse/termleaf/internal/storage"
	"github.com/tolaniverse/termleaf/internal/updater"
	"github.com/tolaniverse/termleaf/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "termleaf:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "version", "--version", "-version":
			fmt.Printf("termleaf %s (%s/%s)\n", version.Current(), runtime.GOOS, runtime.GOARCH)
			return nil
		case "update":
			return runUpdate()
		}
	}

	pageMode := flag.Bool("p", false, "read using terminal-sized pages")
	imageModeFlag := flag.String("images", "auto", "image mode: auto, pixels, or off")
	enableMMDC := flag.Bool("mmdc", false, "allow external mmdc rendering for unsupported Mermaid diagrams")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: termleaf [-p] <document.md>")
		fmt.Fprintln(flag.CommandLine.Output(), "       termleaf version")
		fmt.Fprintln(flag.CommandLine.Output(), "       termleaf update")
		flag.PrintDefaults()
	}
	flag.Parse()
	imageMode, modeErr := render.ParseImageMode(*imageModeFlag)
	if modeErr != nil {
		return modeErr
	}
	if flag.NArg() != 1 {
		flag.Usage()
		return errors.New("expected one Markdown file")
	}

	doc, err := document.Open(flag.Arg(0))
	if err != nil {
		return err
	}

	store, storeErr := storage.Open()
	if storeErr != nil {
		fmt.Fprintln(os.Stderr, "termleaf: position persistence disabled:", storeErr)
	}
	if store != nil {
		defer func() { err = errors.Join(err, store.Close()) }()
	}

	mode := "scroll"
	if *pageMode {
		mode = "page"
	}

	restoredPosition := storage.Position{}
	if store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		restoredPosition, storeErr = store.Load(ctx, doc.ID, mode)
		cancel()
		if storeErr != nil {
			fmt.Fprintln(os.Stderr, "termleaf: could not restore position:", storeErr)
		}
	}

	blocks := document.ResolveImages(document.IndexBlocks(doc.Markdown), doc.Path)
	bookmarks := make([]app.Bookmark, 0)
	if store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		storedBookmarks, bookmarkErr := store.LoadBookmarks(ctx, doc.ID)
		cancel()
		if bookmarkErr != nil {
			fmt.Fprintln(os.Stderr, "termleaf: could not load bookmarks:", bookmarkErr)
		} else {
			for _, bookmark := range storedBookmarks {
				bookmarks = append(bookmarks, app.Bookmark{
					Anchor: page.Anchor{SourceOffset: bookmark.SourceOffset, BlockFraction: bookmark.BlockFraction, Valid: true},
					Label:  bookmark.Label,
				})
			}
		}
	}
	program := tea.NewProgram(app.New(app.Config{
		Name:             doc.Name,
		Markdown:         doc.Markdown,
		PageMode:         *pageMode,
		RestoredLine:     restoredPosition.LineOffset,
		RestoredProgress: restoredPosition.Progress,
		RestoredAnchor: page.Anchor{
			SourceOffset:  restoredPosition.SourceOffset,
			BlockFraction: restoredPosition.BlockFraction,
			Valid:         restoredPosition.AnchorValid,
		},
		Blocks:           blocks,
		ReferenceContext: document.ReferenceContext(doc.Markdown),
		MMDC:             render.FindMMDC(),
		ImageMode:        imageMode,
		EnableMMDC:       *enableMMDC,
		Bookmarks:        bookmarks,
	}))
	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("run reader: %w", err)
	}

	if store != nil {
		model, ok := finalModel.(app.Model)
		if !ok {
			return errors.New("reader returned an unexpected model")
		}
		anchor := model.SemanticAnchor()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		saveErr := store.Save(ctx, doc.ID, mode, storage.Position{
			LineOffset:    model.LineOffset(),
			Progress:      model.ProgressFraction(),
			SourceOffset:  anchor.SourceOffset,
			BlockFraction: anchor.BlockFraction,
			AnchorValid:   anchor.Valid,
		})
		cancel()
		if saveErr != nil {
			return saveErr
		}
		storedBookmarks := make([]storage.Bookmark, 0, len(model.Bookmarks()))
		for _, bookmark := range model.Bookmarks() {
			if !bookmark.Anchor.Valid {
				continue
			}
			storedBookmarks = append(storedBookmarks, storage.Bookmark{
				SourceOffset: bookmark.Anchor.SourceOffset, BlockFraction: bookmark.Anchor.BlockFraction, Label: bookmark.Label,
			})
		}
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		bookmarkErr := store.ReplaceBookmarks(ctx, doc.ID, storedBookmarks)
		cancel()
		if bookmarkErr != nil {
			return bookmarkErr
		}
	}
	return nil
}

func runUpdate() error {
	current := version.Current()
	fmt.Printf("Checking for updates (current: %s)…\n", current)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := updater.Update(ctx, current)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if !result.Updated {
		fmt.Printf("Termleaf %s is already current.\n", result.Current)
		return nil
	}
	fmt.Printf("Updated Termleaf %s → %s.\n", result.Previous, result.Current)
	return nil
}
