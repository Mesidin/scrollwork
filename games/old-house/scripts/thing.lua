function on_death(actor, self)
  echo("The shape folds wrong and slides back into the basin. The water stills.")
end

function on_look(actor, self)
  echo("It notices you noticing it.")
  set_flag("thing", "hostile")
end
