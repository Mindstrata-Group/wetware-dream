"""People written in Latin script."""

import pytest

from tests.helpers import assert_clear, assert_masked


@pytest.mark.parametrize("text,fragment", [
    ("My wife Sarah Johnson says I overreact", "Sarah Johnson"),
    ("My therapist, Dr. Brennan, suggested journaling", "Dr. Brennan"),
    ("Mrs. Hughes from next door called", "Mrs. Hughes"),
    ("Call my sister kate if I don't show up", "kate"),
    ("The kids, Emma and Noah, are with their dad", "Noah"),
    ("Созвонилась с Kate из офиса", "Kate"),
    ("Ivan Petrov уволился", "Ivan Petrov"),
    ("Natasha Ivanova moved abroad", "Natasha Ivanova"),
    ("My boss, Kevin Harrison, said I'm too emotional", "Kevin Harrison"),
])
def test_person_is_masked(text, fragment):
    assert_masked(text, fragment)


@pytest.mark.parametrize("text,fragment", [
    ("I feel sad in May and happy in June.", "May"),
    ("Will you help me with my anxiety?", "Will"),
    ("Grace under pressure is not my thing.", "Grace"),
    ("Hope is the only thing that keeps me going.", "Hope"),
    ("My mom always said I was too sensitive.", "mom"),
])
def test_not_a_person(text, fragment):
    assert_clear(text, fragment)
