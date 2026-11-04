package clipboard

import (
	"errors"
	"strings"
	"testing"
)

// fake is a clipboard tool that holds the given types.
func fake(held map[string]string) runner {
	return func(name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--list-types") || strings.Contains(joined, "TARGETS") {
			var types []string
			for t := range held {
				types = append(types, t)
			}
			return []byte(strings.Join(types, "\n") + "\n"), nil
		}
		t := args[len(args)-1]
		if name == "xclip" {
			t = args[3]
		}
		return []byte(held[t]), nil
	}
}

func wayland(k string) string {
	if k == "WAYLAND_DISPLAY" {
		return "wayland-1"
	}
	return ""
}

func x11(k string) string {
	if k == "DISPLAY" {
		return ":0"
	}
	return ""
}

func TestAPictureWinsOverTextAndPNGOverJPEG(t *testing.T) {
	held := map[string]string{"image/jpeg": "jpg", "image/png": "png", "text/plain;charset=utf-8": "words"}
	for name, env := range map[string]func(string) string{"wayland": wayland, "x11": x11} {
		c, err := read("linux", env, fake(held))
		if err != nil || string(c.Image) != "png" || c.Type != "image/png" || c.Text != "" {
			t.Errorf("%s: got %+v, %v", name, c, err)
		}
	}
}

func TestTextWhenThereIsNoPicture(t *testing.T) {
	c, err := read("linux", wayland, fake(map[string]string{"text/plain;charset=utf-8": "hello", "TEXT": "x"}))
	if err != nil || c.Text != "hello" || c.Image != nil {
		t.Fatalf("got %+v, %v", c, err)
	}
	if _, err := read("linux", wayland, fake(map[string]string{})); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty clipboard: %v", err)
	}
	if _, err := read("linux", func(string) string { return "" }, fake(nil)); err == nil {
		t.Fatal("no display at all was not an error")
	}
}

func TestMacPictureComesAsHex(t *testing.T) {
	run := func(name string, args ...string) ([]byte, error) {
		if name == "osascript" {
			return []byte("«data PNGf89504E47»\n"), nil
		}
		return []byte("text"), nil
	}
	c, err := read("darwin", func(string) string { return "" }, run)
	if err != nil || string(c.Image) != "\x89PNG" {
		t.Fatalf("got %+v, %v", c, err)
	}
	noPicture := func(name string, args ...string) ([]byte, error) {
		if name == "osascript" {
			return nil, errors.New("can't make clipboard into type «class PNGf»")
		}
		return []byte("plain words"), nil
	}
	if c, err := read("darwin", nil, noPicture); err != nil || c.Text != "plain words" {
		t.Fatalf("text on a mac: %+v, %v", c, err)
	}
}

func TestCopiedFilesComeAsTheirAddresses(t *testing.T) {
	c, err := read("linux", wayland, fake(map[string]string{
		"text/uri-list": "file:///home/ada/a%20b.pdf\r\n", "text/plain;charset=utf-8": "a b.pdf",
	}))
	if err != nil || c.Text != "file:///home/ada/a%20b.pdf\r\n" {
		t.Fatalf("got %+v, %v", c, err)
	}
}
